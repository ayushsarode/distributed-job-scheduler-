package api

import (
	"net/http"
)

const uploadFileMaxMemory = 10 << 20

// Handler for /upload-file.
func (r *restApi) UploadFileHandler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	err := req.ParseMultipartForm(uploadFileMaxMemory) // 10 MB
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	file, handler, err := req.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}

	defer func() {
		if err := file.Close(); err != nil {
			r.logger.Error().Err(err).Msg("Failed to close uploaded file")
		}
	}()

	fileType := req.FormValue("file_type")
	fileName := handler.Filename

	err = r.knowledgeBaseService.UploadDocument(req.Context(), file, fileType, fileName)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Error uploading file")
		http.Error(w, "Error uploading file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
