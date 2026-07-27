package deadletterservice

import (
	"encoding/json"
	"net/http"

	"github.com/ayushsarode/distributed-job-scheduler/internal/api/http/dto"
	"github.com/ayushsarode/distributed-job-scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func (s *DeadLetterServiceSuite) TestListGetReplayAndConflict() {
	deadLetterID := s.insertDeadLetter("email", []byte(`{"to":"user@example.com"}`), 3, "timeout")

	listResp := s.doRequest(http.MethodGet, "/dead-letters/?limit=10")
	defer listResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, listResp.StatusCode)

	var listed struct {
		DeadLetters []repository.DeadLetter `json:"dead_letters"`
		Limit       int                     `json:"limit"`
		Offset      int                     `json:"offset"`
	}
	require.NoError(s.T(), json.NewDecoder(listResp.Body).Decode(&listed))
	require.Len(s.T(), listed.DeadLetters, 1)
	require.Equal(s.T(), deadLetterID, listed.DeadLetters[0].ID)
	require.Equal(s.T(), "OPEN", listed.DeadLetters[0].Status)

	getResp := s.doRequest(http.MethodGet, "/dead-letters/"+deadLetterID.String())
	defer getResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, getResp.StatusCode)

	var fetched repository.DeadLetter
	require.NoError(s.T(), json.NewDecoder(getResp.Body).Decode(&fetched))
	require.Equal(s.T(), deadLetterID, fetched.ID)
	require.Equal(s.T(), "email", fetched.JobType)

	replayResp := s.doRequest(http.MethodPost, "/dead-letters/"+deadLetterID.String()+"/replay")
	defer replayResp.Body.Close()
	require.Equal(s.T(), http.StatusCreated, replayResp.StatusCode)

	var replayed struct {
		ReplayedJobID string `json:"replayed_job_id"`
	}
	require.NoError(s.T(), json.NewDecoder(replayResp.Body).Decode(&replayed))
	require.NotEmpty(s.T(), replayed.ReplayedJobID)

	jobResp := s.doRequest(http.MethodGet, "/jobs/"+replayed.ReplayedJobID)
	defer jobResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, jobResp.StatusCode)

	var replayedJob dto.JobResponse
	require.NoError(s.T(), json.NewDecoder(jobResp.Body).Decode(&replayedJob))
	require.Equal(s.T(), "QUEUED", replayedJob.Status)
	require.Equal(s.T(), "email", replayedJob.Type)

	replayedLetterResp := s.doRequest(http.MethodGet, "/dead-letters/"+deadLetterID.String())
	defer replayedLetterResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, replayedLetterResp.StatusCode)

	var replayedLetter repository.DeadLetter
	require.NoError(s.T(), json.NewDecoder(replayedLetterResp.Body).Decode(&replayedLetter))
	require.Equal(s.T(), "REPLAYED", replayedLetter.Status)
	require.NotNil(s.T(), replayedLetter.ReplayedAt)
	require.NotNil(s.T(), replayedLetter.ReplayedJobID)

	conflictResp := s.doRequest(http.MethodPost, "/dead-letters/"+deadLetterID.String()+"/replay")
	defer conflictResp.Body.Close()
	require.Equal(s.T(), http.StatusConflict, conflictResp.StatusCode)
}

func (s *DeadLetterServiceSuite) TestDeleteDeadLetter() {
	deadLetterID := s.insertDeadLetter("cleanup", []byte(`{}`), 1, "failed")

	deleteResp := s.doRequest(http.MethodDelete, "/dead-letters/"+deadLetterID.String())
	defer deleteResp.Body.Close()
	require.Equal(s.T(), http.StatusNoContent, deleteResp.StatusCode)

	getResp := s.doRequest(http.MethodGet, "/dead-letters/"+deadLetterID.String())
	defer getResp.Body.Close()
	require.Equal(s.T(), http.StatusNotFound, getResp.StatusCode)
}

func (s *DeadLetterServiceSuite) TestDeadLetterValidationAndNotFound() {
	invalidResp := s.doRequest(http.MethodGet, "/dead-letters/not-a-uuid")
	defer invalidResp.Body.Close()
	require.Equal(s.T(), http.StatusBadRequest, invalidResp.StatusCode)

	missingResp := s.doRequest(http.MethodGet, "/dead-letters/"+uuid.NewString())
	defer missingResp.Body.Close()
	require.Equal(s.T(), http.StatusNotFound, missingResp.StatusCode)
}

func (s *DeadLetterServiceSuite) insertDeadLetter(jobType string, payload []byte, attempts int, errorMessage string) uuid.UUID {
	s.T().Helper()

	id := uuid.New()
	jobID := uuid.New()
	_, err := s.DB.Pool.Exec(s.T().Context(), `
		INSERT INTO dead_letters (id, job_id, job_type, payload, attempts, error)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, jobID, jobType, payload, attempts, errorMessage)
	require.NoError(s.T(), err)

	return id
}

func (s *DeadLetterServiceSuite) doRequest(method, path string) *http.Response {
	s.T().Helper()

	req, err := http.NewRequestWithContext(s.T().Context(), method, s.ServerURL+path, nil)
	require.NoError(s.T(), err)

	resp, err := s.Client.Do(req)
	require.NoError(s.T(), err)
	return resp
}
