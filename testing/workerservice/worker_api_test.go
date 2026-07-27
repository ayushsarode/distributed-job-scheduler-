package workerservice

import (
	"encoding/json"
	"net/http"

	"github.com/ayushsarode/distributed-job-scheduler/internal/api/http/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func (s *WorkerServiceSuite) TestListAndGetWorkers() {
	activeID := s.insertWorker("worker-active", "ACTIVE", 0.25, 0.40, 2)
	_ = s.insertWorker("worker-idle", "IDLE", 0.10, 0.20, 0)

	listResp := s.doRequest(http.MethodGet, "/workers/?status=ACTIVE")
	defer listResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, listResp.StatusCode)

	var listed dto.ListWorkersResponse
	require.NoError(s.T(), json.NewDecoder(listResp.Body).Decode(&listed))
	require.Len(s.T(), listed.Workers, 1)
	require.Equal(s.T(), activeID.String(), listed.Workers[0].ID)
	require.Equal(s.T(), "worker-active", listed.Workers[0].Host)
	require.Equal(s.T(), "ACTIVE", listed.Workers[0].Status)
	require.Equal(s.T(), 2, listed.Workers[0].RunningJobs)
	require.NotNil(s.T(), listed.Workers[0].CPU)
	require.NotNil(s.T(), listed.Workers[0].Memory)
	require.NotNil(s.T(), listed.Workers[0].LastHeartbeat)

	getResp := s.doRequest(http.MethodGet, "/workers/"+activeID.String())
	defer getResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, getResp.StatusCode)

	var fetched dto.WorkerResponse
	require.NoError(s.T(), json.NewDecoder(getResp.Body).Decode(&fetched))
	require.Equal(s.T(), activeID.String(), fetched.ID)
	require.Equal(s.T(), "worker-active", fetched.Host)
}

func (s *WorkerServiceSuite) TestGetWorkerValidationAndNotFound() {
	invalidResp := s.doRequest(http.MethodGet, "/workers/not-a-uuid")
	defer invalidResp.Body.Close()
	require.Equal(s.T(), http.StatusBadRequest, invalidResp.StatusCode)

	missingResp := s.doRequest(http.MethodGet, "/workers/"+uuid.NewString())
	defer missingResp.Body.Close()
	require.Equal(s.T(), http.StatusNotFound, missingResp.StatusCode)
}

func (s *WorkerServiceSuite) insertWorker(host, status string, cpu, memory float64, runningJobs int) uuid.UUID {
	s.T().Helper()

	id := uuid.New()
	_, err := s.DB.Pool.Exec(s.T().Context(), `
		INSERT INTO workers (id, host, status, cpu, memory, running_jobs, last_heartbeat)
		VALUES ($1, $2, $3, $4, $5, $6, now())
	`, id, host, status, cpu, memory, runningJobs)
	require.NoError(s.T(), err)

	return id
}

func (s *WorkerServiceSuite) doRequest(method, path string) *http.Response {
	s.T().Helper()

	req, err := http.NewRequestWithContext(s.T().Context(), method, s.ServerURL+path, nil)
	require.NoError(s.T(), err)

	resp, err := s.Client.Do(req)
	require.NoError(s.T(), err)
	return resp
}
