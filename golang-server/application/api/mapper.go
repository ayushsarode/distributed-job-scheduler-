package api

import (
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
)

func mapToDeploymentStatus(status entity.DeploymentStatus) pb.DeploymentStatus {
	switch status {
	case entity.DeploymentStatusNotDeployed:
		return pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED
	case entity.DeploymentStatusInProgress:
		return pb.DeploymentStatus_DEPLOYMENT_STATUS_IN_PROGRESS
	case entity.DeploymentStatusSuccess:
		return pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS
	case entity.DeploymentStatusFailed:
		return pb.DeploymentStatus_DEPLOYMENT_STATUS_FAILED
	case entity.DeploymentStatusUnknown:
		return pb.DeploymentStatus_DEPLOYMENT_STATUS_UNKNOWN
	default:
		return pb.DeploymentStatus_DEPLOYMENT_STATUS_UNKNOWN
	}
}

func mapToEntityDeploymentStatus(status pb.DeploymentStatus) entity.DeploymentStatus {
	switch status {
	case pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED:
		return entity.DeploymentStatusNotDeployed
	case pb.DeploymentStatus_DEPLOYMENT_STATUS_IN_PROGRESS:
		return entity.DeploymentStatusInProgress
	case pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS:
		return entity.DeploymentStatusSuccess
	case pb.DeploymentStatus_DEPLOYMENT_STATUS_FAILED:
		return entity.DeploymentStatusFailed
	case pb.DeploymentStatus_DEPLOYMENT_STATUS_UNKNOWN:
		return entity.DeploymentStatusUnknown
	default:
		return entity.DeploymentStatusUnknown
	}
}

func mapToCallJobStatus(status entity.CallJobStatus) pb.CallJobStatus {
	switch status {
	case entity.CallJobPending:
		return pb.CallJobStatus_CALL_JOB_STATUS_PENDING
	case entity.CallJobProcessing:
		return pb.CallJobStatus_CALL_JOB_STATUS_PROCESSING
	case entity.CallJobReady:
		return pb.CallJobStatus_CALL_JOB_STATUS_READY
	case entity.CallJobRunning:
		return pb.CallJobStatus_CALL_JOB_STATUS_RUNNING
	case entity.CallJobCompleted:
		return pb.CallJobStatus_CALL_JOB_STATUS_COMPLETED
	case entity.CallJobFailed:
		return pb.CallJobStatus_CALL_JOB_STATUS_FAILED
	case entity.CallJobUnknown:
		return pb.CallJobStatus_CALL_JOB_STATUS_UNKNOWN
	default:
		return pb.CallJobStatus_CALL_JOB_STATUS_UNKNOWN
	}
}

func mapToEntityCallJobStatus(status pb.CallJobStatus) entity.CallJobStatus {
	switch status {
	case pb.CallJobStatus_CALL_JOB_STATUS_PENDING:
		return entity.CallJobPending
	case pb.CallJobStatus_CALL_JOB_STATUS_PROCESSING:
		return entity.CallJobProcessing
	case pb.CallJobStatus_CALL_JOB_STATUS_READY:
		return entity.CallJobReady
	case pb.CallJobStatus_CALL_JOB_STATUS_RUNNING:
		return entity.CallJobRunning
	case pb.CallJobStatus_CALL_JOB_STATUS_COMPLETED:
		return entity.CallJobCompleted
	case pb.CallJobStatus_CALL_JOB_STATUS_FAILED:
		return entity.CallJobFailed
	case pb.CallJobStatus_CALL_JOB_STATUS_UNKNOWN:
		return entity.CallJobUnknown
	default:
		return entity.CallJobUnknown
	}
}

//nolint:cyclop
func mapToCallJobItemStatus(status entity.CallJobItemStatus) pb.CallJobItemStatus {
	switch status {
	case entity.CallJobItemStatusPending:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_PENDING
	case entity.CallJobItemStatusInitiated:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_INITIATED
	case entity.CallJobItemStatusQueued:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_QUEUED
	case entity.CallJobItemStatusRinging:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_RINGING
	case entity.CallJobItemStatusAnswered:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_ANSWERED
	case entity.CallJobItemStatusCompleted:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_COMPLETED
	case entity.CallJobItemStatusBusy:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_BUSY
	case entity.CallJobItemStatusFailed:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_FAILED
	case entity.CallJobItemStatusNoAnswer:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_NO_ANSWER
	case entity.CallJobItemStatusCanceled:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_CANCELED
	case entity.CallJobItemStatusUnknown:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_UNKNOWN
	default:
		return pb.CallJobItemStatus_CALL_JOB_ITEM_STATUS_UNKNOWN
	}
}

func mapToCredentialType(status entity.CredentialType) pb.CredentialType {
	switch status {
	case entity.CredentialTypeTwilio:
		return pb.CredentialType_TWILIO
	case entity.CredentialTypeExotel:
		return pb.CredentialType_EXOTEL
	case entity.CredentialTypeUnspecified:
		return pb.CredentialType_CREDENTIAL_TYPE_UNSPECIFIED
	default:
		return pb.CredentialType_CREDENTIAL_TYPE_UNSPECIFIED
	}
}

func mapToEntityCredentialType(status pb.CredentialType) entity.CredentialType {
	switch status {
	case pb.CredentialType_TWILIO:
		return entity.CredentialTypeTwilio
	case pb.CredentialType_EXOTEL:
		return entity.CredentialTypeExotel
	case pb.CredentialType_CREDENTIAL_TYPE_UNSPECIFIED:
		return entity.CredentialTypeUnspecified
	default:
		return entity.CredentialTypeUnspecified
	}
}

func mapToWorkflowStatus(status entity.WorkflowStatus) pb.WorkflowStatus {
	switch status {
	case entity.WorkflowDraft:
		return pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT
	case entity.WorkflowPublished:
		return pb.WorkflowStatus_WORKFLOW_STATUS_PUBLISHED
	case entity.WorkflowInActive:
		return pb.WorkflowStatus_WORKFLOW_STATUS_INACTIVE
	case entity.WorkflowUnknown:
		return pb.WorkflowStatus_WORKFLOW_STATUS_UNKNOWN
	default:
		return pb.WorkflowStatus_WORKFLOW_STATUS_UNKNOWN
	}
}

func mapToEntityWorkflowStatus(status pb.WorkflowStatus) entity.WorkflowStatus {
	switch status {
	case pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT:
		return entity.WorkflowDraft
	case pb.WorkflowStatus_WORKFLOW_STATUS_PUBLISHED:
		return entity.WorkflowPublished
	case pb.WorkflowStatus_WORKFLOW_STATUS_INACTIVE:
		return entity.WorkflowInActive
	case pb.WorkflowStatus_WORKFLOW_STATUS_UNKNOWN:
		return entity.WorkflowUnknown
	default:
		return entity.WorkflowUnknown
	}
}
