package types

import (
	"time"

	"gorm.io/gorm"
)

// EvaluationTaskEntity is the database representation of one evaluation task.
// API serialization continues to use EvaluationTask and EvaluationDetail.
type EvaluationTaskEntity struct {
	ID        string           `json:"id" gorm:"type:varchar(128);primaryKey"`
	TenantID  uint64           `json:"tenant_id" gorm:"not null;index"`
	DatasetID string           `json:"dataset_id" gorm:"type:varchar(255);not null"`
	Status    EvaluationStatue `json:"status" gorm:"not null;index"`
	StartTime time.Time        `json:"start_time" gorm:"not null"`
	EndTime   *time.Time       `json:"end_time,omitempty"`
	Total     int              `json:"total" gorm:"not null;default:0"`
	Finished  int              `json:"finished" gorm:"not null;default:0"`
	ErrMsg    string           `json:"err_msg" gorm:"type:text;not null;default:''"`
	DeletedAt gorm.DeletedAt   `json:"-" gorm:"index"`
	CreatedAt time.Time        `json:"created_at" gorm:"not null"`
	UpdatedAt time.Time        `json:"updated_at" gorm:"not null"`

	CleanupErrors JSON `json:"cleanup_errors" gorm:"type:jsonb;not null;default:'[]'"`
	Params        JSON `json:"params" gorm:"type:jsonb;not null;default:'{}'"`
	Metric        JSON `json:"metric,omitempty" gorm:"type:jsonb"`

	TemporaryKnowledgeBaseID string `json:"-" gorm:"column:temporary_kb_id;type:varchar(64);not null"`
	TemporaryKnowledgeID     string `json:"-" gorm:"type:varchar(64)"`

	OwnerID        string     `json:"-" gorm:"type:varchar(36);not null"`
	LeaseExpiresAt *time.Time `json:"-" gorm:"index"`
	HeartbeatAt    time.Time  `json:"-" gorm:"not null"`
	Version        uint64     `json:"version" gorm:"not null;default:1"`
}

// TableName binds EvaluationTaskEntity to the evaluation task table.
func (EvaluationTaskEntity) TableName() string {
	return "evaluation_tasks"
}

// EvaluationTaskStartCommand conditionally moves a pending task into the
// running state for its current owner.
type EvaluationTaskStartCommand struct {
	TenantID        uint64
	TaskID          string
	OwnerID         string
	ExpectedVersion uint64
	Now             time.Time
	LeaseExpiresAt  time.Time
}

// EvaluationTaskProgressCommand conditionally publishes one complete progress
// snapshot and renews the running task lease.
type EvaluationTaskProgressCommand struct {
	TenantID        uint64
	TaskID          string
	OwnerID         string
	ExpectedVersion uint64
	Total           int
	Finished        int
	Metric          JSON
	Now             time.Time
	LeaseExpiresAt  time.Time
}

// EvaluationTaskKnowledgeCommand conditionally records the temporary
// Knowledge resource created by a running task.
type EvaluationTaskKnowledgeCommand struct {
	TenantID             uint64
	TaskID               string
	OwnerID              string
	ExpectedVersion      uint64
	TemporaryKnowledgeID string
	UpdatedAt            time.Time
}

// EvaluationTaskTerminalCommand conditionally publishes the complete terminal
// snapshot and releases the task lease.
type EvaluationTaskTerminalCommand struct {
	TenantID        uint64
	TaskID          string
	OwnerID         string
	ExpectedVersion uint64
	Status          EvaluationStatue
	EndTime         time.Time
	ErrMsg          string
	CleanupErrors   JSON
	Metric          JSON
}
