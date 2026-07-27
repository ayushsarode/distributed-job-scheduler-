package jobservice

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/ayushsarode/distributed-job-scheduler/internal/api/http/dto"
	"github.com/stretchr/testify/require"
)

func (s *JobServiceSuite) TestSubmitGetAndListJob() {
	body := []byte(`{"type":"email","payload":{"to":"user@example.com"},"priority":7}`)

	resp := s.doJSON(http.MethodPost, "/jobs/", body, "")
	defer resp.Body.Close()
	require.Equal(s.T(), http.StatusCreated, resp.StatusCode)

	var created dto.JobResponse
	require.NoError(s.T(), json.NewDecoder(resp.Body).Decode(&created))
	require.NotEmpty(s.T(), created.ID)
	require.Equal(s.T(), "QUEUED", created.Status)
	require.Equal(s.T(), "email", created.Type)
	require.Equal(s.T(), int16(7), created.Priority)

	getResp := s.doJSON(http.MethodGet, "/jobs/"+created.ID, nil, "")
	defer getResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, getResp.StatusCode)
	require.Equal(s.T(), "MISS", getResp.Header.Get("X-Cache"))

	var fetched dto.JobResponse
	require.NoError(s.T(), json.NewDecoder(getResp.Body).Decode(&fetched))
	require.Equal(s.T(), created.ID, fetched.ID)

	cachedResp := s.doJSON(http.MethodGet, "/jobs/"+created.ID, nil, "")
	defer cachedResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, cachedResp.StatusCode)
	require.Equal(s.T(), "HIT", cachedResp.Header.Get("X-Cache"))

	listResp := s.doJSON(http.MethodGet, "/jobs/?status=QUEUED&type=email", nil, "")
	defer listResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, listResp.StatusCode)

	var listed dto.ListJobsResponse
	require.NoError(s.T(), json.NewDecoder(listResp.Body).Decode(&listed))
	require.Len(s.T(), listed.Jobs, 1)
	require.Equal(s.T(), created.ID, listed.Jobs[0].ID)
}

func (s *JobServiceSuite) TestSubmitJobIsIdempotent() {
	body := []byte(`{"type":"report","payload":{"tenant":"acme"},"priority":3}`)

	first := s.doJSON(http.MethodPost, "/jobs/", body, "same-request")
	defer first.Body.Close()
	require.Equal(s.T(), http.StatusCreated, first.StatusCode)

	var firstJob dto.JobResponse
	require.NoError(s.T(), json.NewDecoder(first.Body).Decode(&firstJob))

	second := s.doJSON(http.MethodPost, "/jobs/", body, "same-request")
	defer second.Body.Close()
	require.Equal(s.T(), http.StatusOK, second.StatusCode)

	var secondJob dto.JobResponse
	require.NoError(s.T(), json.NewDecoder(second.Body).Decode(&secondJob))
	require.Equal(s.T(), firstJob.ID, secondJob.ID)

	mismatched := s.doJSON(http.MethodPost, "/jobs/", []byte(`{"type":"report","payload":{"tenant":"other"},"priority":3}`), "same-request")
	defer mismatched.Body.Close()
	require.Equal(s.T(), http.StatusConflict, mismatched.StatusCode)
}

func (s *JobServiceSuite) TestCancelJob() {
	resp := s.doJSON(http.MethodPost, "/jobs/", []byte(`{"type":"cleanup","payload":{},"priority":1}`), "")
	defer resp.Body.Close()
	require.Equal(s.T(), http.StatusCreated, resp.StatusCode)

	var created dto.JobResponse
	require.NoError(s.T(), json.NewDecoder(resp.Body).Decode(&created))

	cancelResp := s.doJSON(http.MethodDelete, "/jobs/"+created.ID, nil, "")
	defer cancelResp.Body.Close()
	require.Equal(s.T(), http.StatusNoContent, cancelResp.StatusCode)

	getResp := s.doJSON(http.MethodGet, "/jobs/"+created.ID, nil, "")
	defer getResp.Body.Close()
	require.Equal(s.T(), http.StatusOK, getResp.StatusCode)

	var fetched dto.JobResponse
	require.NoError(s.T(), json.NewDecoder(getResp.Body).Decode(&fetched))
	require.Equal(s.T(), "DEAD", fetched.Status)
}

func (s *JobServiceSuite) doJSON(method, path string, body []byte, idemKey string) *http.Response {
	s.T().Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(s.T().Context(), method, s.ServerURL+path, reader)
	require.NoError(s.T(), err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}

	resp, err := s.Client.Do(req)
	require.NoError(s.T(), err)
	return resp
}
