package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (g *grpcApi) SetCredential(
	ctx context.Context,
	req *connect.Request[pb.SetCredentialRequest],
) (*connect.Response[pb.SetCredentialResponse], error) {
	if req.Msg.GetCredentialName() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("credential_name is required"))
	}

	if req.Msg.GetCredential() == nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("credential is required"))
	}

	credential := entity.Credential{
		Credential:         req.Msg.GetCredential(),
		CredentialMetadata: req.Msg.GetCredentialMetadata(),
	}

	credentialEncrypted, err := g.credentialService.SetCredential(
		ctx,
		req.Msg.GetCredentialName(),
		credential,
		mapToEntityCredentialType(req.Msg.GetCredentialType()),
	)
	if err != nil {
		return nil, err
	}

	resp := &pb.SetCredentialResponse{
		CredentialId:   credentialEncrypted.ID.String(),
		CredentialName: credentialEncrypted.CredentialName,
		CredentialType: mapToCredentialType(credentialEncrypted.Type),
		CreatedBy:      credentialEncrypted.CreatedBy,
		CreatedAt:      timestamppb.New(credentialEncrypted.CreatedAt),
		UpdatedAt:      timestamppb.New(credentialEncrypted.UpdatedAt),
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) GetCredential(
	ctx context.Context,
	req *connect.Request[pb.GetCredentialRequest],
) (*connect.Response[pb.GetCredentialResponse], error) {
	credentialId, err := uuid.Parse(req.Msg.GetCredentialId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("credential_id must be a valid UUID"))
	}

	if credentialId == uuid.Nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("credential_id is required"))
	}

	credential, err := g.credentialService.GetCredential(ctx, credentialId)
	if err != nil {
		return nil, err
	}

	resp := &pb.GetCredentialResponse{
		CredentialId:       credential.ID.String(),
		CredentialName:     credential.CredentialName,
		Type:               mapToCredentialType(credential.Type),
		CredentialMetadata: credential.CredentialMetadata,
		CreatedAt:          timestamppb.New(credential.CreatedAt),
		UpdatedAt:          timestamppb.New(credential.UpdatedAt),
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) DeleteCredential(
	ctx context.Context,
	req *connect.Request[pb.DeleteCredentialRequest],
) (*connect.Response[emptypb.Empty], error) {
	if req.Msg.GetCredentialId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("credential_id is required"))
	}

	credentialId, err := uuid.Parse(req.Msg.GetCredentialId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("credential_id must be a valid UUID"))
	}

	err = g.credentialService.DeleteCredential(ctx, credentialId)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) ListCredentials(
	ctx context.Context,
	req *connect.Request[pb.ListCredentialsRequest],
) (*connect.Response[pb.ListCredentialsResponse], error) {
	types := g.parseCredentialTypeFilter(req.Msg.GetType())

	credentials, err := g.credentialService.ListCredentials(ctx, types, req.Msg.GetLimit(), req.Msg.GetOffset())
	if err != nil {
		return nil, err
	}

	resp := &pb.ListCredentialsResponse{
		Credentials: make([]*pb.GetCredentialResponse, 0, len(credentials)),
	}

	for _, cred := range credentials {
		resp.Credentials = append(resp.Credentials, &pb.GetCredentialResponse{
			CredentialId:       cred.ID.String(),
			CredentialName:     cred.CredentialName,
			Type:               mapToCredentialType(cred.Type),
			CredentialMetadata: cred.CredentialMetadata,
			CreatedAt:          timestamppb.New(cred.CreatedAt),
			UpdatedAt:          timestamppb.New(cred.UpdatedAt),
		})
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) parseCredentialTypeFilter(statuses []pb.CredentialType) []entity.CredentialType {
	statusFilter := make([]entity.CredentialType, 0, len(statuses))

	if len(statuses) >= 1 {
		for _, statusItem := range statuses {
			if statusItem != pb.CredentialType_CREDENTIAL_TYPE_UNSPECIFIED {
				parsedStatus := mapToEntityCredentialType(statusItem)
				statusFilter = append(statusFilter, parsedStatus)
			}
		}
	}

	return statusFilter
}
