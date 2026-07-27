package agentutils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"github.com/oklog/ulid/v2"
)

func GenerateAgentID(ctx context.Context, deploymentType pb.DeploymentType, model pb.AgentModel) (string, error) {
	if model == pb.AgentModel_AGENTMODEL_UNSPECIFIED {
		return "", xerrors.BadRequestError(ctx, errors.New("model must be explicitly specified"))
	}
	
	if deploymentType == pb.DeploymentType_DEPLOYMENT_TYPE_RT && model != pb.AgentModel_AGENTMODEL_GPT_REALTIME {
		return "", xerrors.BadRequestError(ctx, errors.New("RT deployment type requires GPT_REALTIME model"))
	}

	if model == pb.AgentModel_AGENTMODEL_GPT_REALTIME && deploymentType != pb.DeploymentType_DEPLOYMENT_TYPE_RT {
		return "", xerrors.BadRequestError(ctx, errors.New("GPT_REALTIME model requires RT deployment type"))
	}

	prefix, err := getSuffix(ctx, deploymentType)
	if err != nil {
		return "", err
	}
	return ulid.Make().String() + prefix, nil
}

func getSuffix(ctx context.Context, deploymentType pb.DeploymentType) (string, error) {
	switch deploymentType {
	case pb.DeploymentType_DEPLOYMENT_TYPE_RT:
		return "_rt", nil
	case pb.DeploymentType_DEPLOYMENT_TYPE_WS:
		return "_ws", nil
	case pb.DeploymentType_DEPLOYMENT_TYPE_LG:
		return "_lg", nil
	case pb.DeploymentType_DEPLOYMENT_TYPE_UNKNOWN:
		return "", xerrors.BadRequestError(ctx, errors.New("deployment_type must be explicitly specified (LG, WS, or RT)"))
	default:
		return "", xerrors.BadRequestError(ctx, fmt.Errorf("unsupported deployment type: %v", deploymentType))
	}
}

func IsLGAgent(id string) bool { return strings.HasSuffix(id, "_lg") }
func IsWSAgent(id string) bool { return strings.HasSuffix(id, "_ws") }
func IsRTAgent(id string) bool { return strings.HasSuffix(id, "_rt") }

func ValidateAgentSuffixCompatibility(ctx context.Context, existingAgentID string, deploymentType pb.DeploymentType, model pb.AgentModel) error {
	newAgentID, err := GenerateAgentID(ctx, deploymentType, model)
	if err != nil {
		return err
	}

	existingSuffix := extractSuffix(existingAgentID)
	newSuffix := extractSuffix(newAgentID)

	if existingSuffix == "" || newSuffix == "" {
		return xerrors.BadRequestError(ctx, errors.New("invalid agent ID format"))
	}

	if existingSuffix != newSuffix {
		return xerrors.BadRequestError(ctx, errors.New("cannot change deployment type or model in a way that changes agent suffix"))
	}

	return nil
}

func extractSuffix(agentID string) string {
	idx := strings.LastIndex(agentID, "_")
	if idx == -1 {
		return ""
	}
	return agentID[idx:]
}
