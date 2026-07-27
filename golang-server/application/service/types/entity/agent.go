package entity

import (
	"errors"
	"fmt"
	"time"

	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// DeploymentStatus represents the deployment status of an agent.
type DeploymentStatus int

const (
	DeploymentStatusNotDeployed DeploymentStatus = iota
	DeploymentStatusInProgress
	DeploymentStatusSuccess
	DeploymentStatusFailed
	DeploymentStatusUnknown
)

// String returns the string representation of the DeploymentStatus.
func (s DeploymentStatus) String() string {
	switch s {
	case DeploymentStatusNotDeployed:
		return "NotDeployed"
	case DeploymentStatusInProgress:
		return "InProgress"
	case DeploymentStatusSuccess:
		return "Success"
	case DeploymentStatusFailed:
		return "Failed"
	case DeploymentStatusUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

func ParseDeploymentStatus(status string) (DeploymentStatus, error) {
	switch status {
	case "NotDeployed":
		return DeploymentStatusNotDeployed, nil
	case "InProgress":
		return DeploymentStatusInProgress, nil
	case "Success":
		return DeploymentStatusSuccess, nil
	case "Failed":
		return DeploymentStatusFailed, nil
	case "Unknown":
		return DeploymentStatusUnknown, nil
	default:
		return DeploymentStatusNotDeployed, fmt.Errorf("invalid deployment status: %s", status)
	}
}

type RealtimeEventType int

const (
	Unknown RealtimeEventType = iota
	SessionCreated
	AudioDelta
	AudioTranscriptDone
	SpeechStarted
	TranscriptionCompleted
	FunctionCall
	Error
)

func (t RealtimeEventType) String() string {
	switch t {
	case Unknown:
		return "Unknown"
	case SessionCreated:
		return "SessionCreated"
	case AudioDelta:
		return "AudioDelta"
	case AudioTranscriptDone:
		return "AudioTranscriptDone"
	case SpeechStarted:
		return "SpeechStarted"
	case TranscriptionCompleted:
		return "TranscriptionCompleted"
	case FunctionCall:
		return "FunctionCall"
	case Error:
		return "Error"
	default:
		return "Unknown"
	}
}

type Agent struct {
	ID                 string
	AgentName          string
	CreateAgentRequest *pb.CreateAgentRequest
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatedBy          string
	TenantID           uuid.UUID
	DeploymentStatus   DeploymentStatus
}

func (a *Agent) GetModel() (pb.AgentModel, error) {
	if a.CreateAgentRequest == nil {
		return pb.AgentModel_AGENTMODEL_UNSPECIFIED, errors.New("agent has no create request")
	}

	req := a.CreateAgentRequest.GetAgentPromptRequest()
	if req == nil {
		return pb.AgentModel_AGENTMODEL_UNSPECIFIED, errors.New("agent template requests are not supported")
	}

	model := req.GetModel()
	if model == pb.AgentModel_AGENTMODEL_UNSPECIFIED {
		return pb.AgentModel_AGENTMODEL_UNSPECIFIED, errors.New("agent model is unspecified")
	}

	return model, nil
}
