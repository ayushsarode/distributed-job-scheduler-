package knowledgebase

import (
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres/gen"
)

func (r *Repository) mapToDocument(doc gen.Document) entity.Document {
	return entity.Document{
		ID:           doc.ID,
		DocumentName: doc.DocumentName,
		DocumentUrl:  doc.DocumentUrl,
		IsIndexed:    doc.IsIndexed,
		IsActive:     doc.IsActive,
		IsDeleted:    doc.IsDeleted,
		CreatedBy:    doc.CreatedBy,
		TenantID:     doc.TenantID,
		CreatedAt:    doc.CreatedAt.Time,
		UpdatedAt:    doc.UpdatedAt.Time,
	}
}
