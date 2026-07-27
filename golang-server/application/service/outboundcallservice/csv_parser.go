package outboundcallservice

import (
	"encoding/csv"
	"fmt"
	"io"
	"errors"

	"exiro.ai/application/service/types/entity"
)

const (
	minCSVRecordLength = 2
)

// parseCSV parses CSV file content and returns job items.
func (s *Service) parseCSV(file io.Reader) ([]entity.JobItem, error) {
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	// Skip the header
	if len(records) < 1 {
		return nil, errors.New("CSV is empty")
	}

	data := make([]entity.JobItem, 0, len(records)-1)
	for i, record := range records {
		if i == 0 {
			continue // Skip header row
		}
		if len(record) < minCSVRecordLength {
			s.logger.Warn().Int("row", i).Msg("Skipping CSV row with insufficient columns")
			continue
		}
		data = append(data, entity.JobItem{
			PhoneNumber:  record[0],
			AgentContext: record[1],
		})
	}

	s.logger.Debug().Int("rows_parsed", len(data)).Msg("CSV parsing completed")
	return data, nil
}
