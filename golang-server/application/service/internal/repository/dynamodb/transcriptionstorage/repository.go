package transcriptionstorage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/rs/zerolog"
)

type TranscriptionRepository struct {
	tableName   string
	client      *dynamodb.Client
	logger      *zerolog.Logger
}

func NewTranscriptionRepository(ctx context.Context) *TranscriptionRepository {
	return &TranscriptionRepository{
		tableName: config.Ctx(ctx).TranscriptionStorage.TranscriptionTableName,
		client:    NewDynamoDBClient(ctx),
		logger:    zerolog.Ctx(ctx),
	}
}

func (r *TranscriptionRepository) InsertSegment(ctx context.Context, event *pb.TranscriptEvent, segment *pb.Segment) error {
	segmentItem := entity.SegmentItem{
		SessionId:    event.GetSessionId(),
		SortKey:      fmt.Sprintf("SEG#%06d", segment.GetSequence()),
		SegmentId:    event.GetSegmentId(),
		Sequence:     segment.GetSequence(),
		Speaker:      segment.GetSpeaker().String(),
		Text:         segment.GetText(),
		Timestamp:    segment.GetTimestamp().AsTime(),
		LanguageCode: segment.GetLanguageCode(),
	}

	segAv, err := attributevalue.MarshalMap(segmentItem)
	if err != nil {
		return xerrors.InternalError(ctx, err)
	}

	putSeg := dynamodbTypes.TransactWriteItem{
		Put: &dynamodbTypes.Put{
			TableName:           &r.tableName,
			Item:                segAv,
			ConditionExpression: aws.String("attribute_not_exists(segment_id)"),
		},
	}

	updateMeta, err := r.buildMetaUpdateItem(ctx, event.GetSessionId(), event.GetTenantId())
	if err != nil {
		return err
	}

	_, err = r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []dynamodbTypes.TransactWriteItem{putSeg, updateMeta},
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *TranscriptionRepository) ClaimSession(ctx context.Context, sessionID string) error {
	now := time.Now().Unix()

	key := entity.MetaKey{
		SessionID: sessionID,
		SortKey:   "META",
	}
	keyAv, err := attributevalue.MarshalMap(key)
	if err != nil {
		return xerrors.InternalError(ctx, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &r.tableName,
		Key:                 keyAv,
		UpdateExpression:    aws.String("SET #st = :agg, #last_updated = :now"),
		ConditionExpression: aws.String("#st = :inprog"),
		ExpressionAttributeNames: map[string]string{
			"#st":           "status",
			"#last_updated": "last_updated",
		},
		ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
			":inprog": &dynamodbTypes.AttributeValueMemberS{Value: "IN_PROGRESS"},
			":agg":    &dynamodbTypes.AttributeValueMemberS{Value: "AGGREGATING"},
			":now":    &dynamodbTypes.AttributeValueMemberN{Value: strconv.FormatInt(now, 10)},
		},
	})
	if err != nil {
		var cfe *dynamodbTypes.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			return nil
		}
		return xerrors.InternalError(ctx, err)
	}

	return nil
}

func (r *TranscriptionRepository) GetSessionMeta(ctx context.Context, sessionID string) (*entity.MetadataDynamo, error) {
	key := entity.MetaKey{
		SessionID: sessionID,
		SortKey:   "META",
	}
	keyAv, err := attributevalue.MarshalMap(key)
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key:       keyAv,
	})
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}
	if out.Item == nil {
		return nil, xerrors.InternalError(ctx, errors.New("session meta not found"))
	}

	var metaDynamo entity.MetadataDynamo
	if err := attributevalue.UnmarshalMap(out.Item, &metaDynamo); err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	return &metaDynamo, nil
}

func (r *TranscriptionRepository) FetchSessionItems(ctx context.Context, sessionID string) (entity.MetaItem, []entity.SegmentItem, error) {
	qOut, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &r.tableName,
		KeyConditionExpression: aws.String("#pk = :session"),
		ExpressionAttributeNames: map[string]string{
			"#pk": "session_id",
		},
		ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
			":session": &dynamodbTypes.AttributeValueMemberS{Value: sessionID},
		},
	})
	if err != nil {
		return entity.MetaItem{}, nil, xerrors.InternalError(ctx, err)
	}

	var meta entity.MetaItem
	var segments []entity.SegmentItem

	for _, avMap := range qOut.Items {
		if err := r.processSessionItem(avMap, &meta, &segments); err != nil {
			return entity.MetaItem{}, nil, err
		}
	}

	return meta, segments, nil
}

func (r *TranscriptionRepository) FinalizeSession(ctx context.Context, sessionID, s3Key, status string, duration float64) error {
	finalKey := entity.MetaKey{
		SessionID: sessionID,
		SortKey:   "META",
	}
	finalKeyAv, err := attributevalue.MarshalMap(finalKey)
	if err != nil {
		return xerrors.InternalError(ctx, err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:        &r.tableName,
		Key:              finalKeyAv,
		UpdateExpression: aws.String("SET #status = :st, #last_updated = :now, #s3_key = :s3key, #duration = :dur"),
		ExpressionAttributeNames: map[string]string{
			"#status":       "status",
			"#last_updated": "last_updated",
			"#s3_key":       "s3_key",
			"#duration":     "duration_secs",
		},
		ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
			":st":    &dynamodbTypes.AttributeValueMemberS{Value: status},
			":now":   &dynamodbTypes.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Unix(), 10)},
			":s3key": &dynamodbTypes.AttributeValueMemberS{Value: s3Key},
			":dur":   &dynamodbTypes.AttributeValueMemberN{Value: fmt.Sprintf("%f", duration)},
		},
	})
	if err != nil {
		return xerrors.InternalError(ctx, err)
	}

	return nil
}

func (r *TranscriptionRepository) Query(ctx context.Context, status string, olderThan time.Time) ([]string, error) {
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &r.tableName,
		IndexName:              aws.String("StatusLastUpdatedIndex"),
		KeyConditionExpression: aws.String("#st = :st AND #lu < :cut"),
		ExpressionAttributeNames: map[string]string{
			"#st": "status",
			"#lu": "last_updated",
		},
		ExpressionAttributeValues: map[string]dynamodbTypes.AttributeValue{
			":st":  &dynamodbTypes.AttributeValueMemberS{Value: status},
			":cut": &dynamodbTypes.AttributeValueMemberN{Value: strconv.FormatInt(olderThan.Unix(), 10)},
		},
		ProjectionExpression: aws.String("session_id"),
	})
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	var sessionIDs []string
	for _, item := range out.Items {
		if sidAttr, ok := item["session_id"].(*dynamodbTypes.AttributeValueMemberS); ok {
			sessionIDs = append(sessionIDs, sidAttr.Value)
		}
	}
	return sessionIDs, nil
}


func (r *TranscriptionRepository) buildMetaUpdateItem(ctx context.Context, sessionID, tenantID string) (dynamodbTypes.TransactWriteItem, error) {
	now := time.Now().Unix()

	key := entity.MetaKey{
		SessionID: sessionID,
		SortKey:   "META",
	}
	keyMeta, err := attributevalue.MarshalMap(key)
	if err != nil {
		return dynamodbTypes.TransactWriteItem{}, xerrors.InternalError(ctx, err)
	}

	update := expression.
		Set(expression.Name("last_updated"), expression.Value(now)).
		Set(expression.Name("created_at"), expression.IfNotExists(expression.Name("created_at"), expression.Value(now))).
		Set(expression.Name("status"), expression.IfNotExists(expression.Name("status"), expression.Value("IN_PROGRESS"))).
		Set(expression.Name("owner_id"), expression.IfNotExists(expression.Name("owner_id"), expression.Value(tenantID)))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return dynamodbTypes.TransactWriteItem{}, xerrors.InternalError(ctx, err)
	}

	return dynamodbTypes.TransactWriteItem{
		Update: &dynamodbTypes.Update{
			TableName:                 &r.tableName,
			Key:                       keyMeta,
			UpdateExpression:          expr.Update(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
		},
	}, nil
}

func (r *TranscriptionRepository) processSessionItem(avMap map[string]dynamodbTypes.AttributeValue, meta *entity.MetaItem, segments *[]entity.SegmentItem) error {
	skAttr, ok := avMap["sort_key"].(*dynamodbTypes.AttributeValueMemberS)
	if !ok {
		return nil
	}

	if skAttr.Value == "META" {
		return r.unmarshalMetaItem(avMap, meta)
	}

	if strings.HasPrefix(skAttr.Value, "SEG#") {
		return r.unmarshalSegmentItem(avMap, segments)
	}

	return nil
}

func (r *TranscriptionRepository) unmarshalMetaItem(avMap map[string]dynamodbTypes.AttributeValue, meta *entity.MetaItem) error {
	if err := attributevalue.UnmarshalMap(avMap, meta); err != nil {
		return err
	}
	return nil
}

func (r *TranscriptionRepository) unmarshalSegmentItem(avMap map[string]dynamodbTypes.AttributeValue, segments *[]entity.SegmentItem) error {
	var seg entity.SegmentItem
	if err := attributevalue.UnmarshalMap(avMap, &seg); err != nil {
		return err
	}
	*segments = append(*segments, seg)
	return nil
}
