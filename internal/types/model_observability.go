package types

import (
	"time"

	"gorm.io/gorm"
)

const (
	ModelCallStatusStarted  = "started"
	ModelCallStatusSuccess  = "success"
	ModelCallStatusError    = "error"
	ModelCallStatusCanceled = "canceled"
)

const (
	ApplicationCacheStatusUnavailable = "unavailable"
	ApplicationCacheStatusBypass      = "bypass"
	ApplicationCacheStatusMiss        = "miss"
)

// ModelCallRecord is one provider-call ledger row without prompts or business content.
type ModelCallRecord struct {
	ID                         string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID                   uint64         `json:"tenant_id" gorm:"not null;index"`
	EvaluationTaskID           string         `json:"evaluation_task_id,omitempty" gorm:"type:varchar(128);not null;default:'';index"`
	ModelID                    string         `json:"model_id" gorm:"type:varchar(64);not null;index"`
	ModelSnapshot              JSON           `json:"model_snapshot" gorm:"type:jsonb;not null"`
	Purpose                    string         `json:"purpose" gorm:"type:varchar(64);not null"`
	Operation                  string         `json:"operation" gorm:"type:varchar(32);not null"`
	StartedAt                  time.Time      `json:"started_at" gorm:"not null;index"`
	EndedAt                    *time.Time     `json:"ended_at,omitempty"`
	DurationMs                 *int64         `json:"duration_ms,omitempty"`
	Status                     string         `json:"status" gorm:"type:varchar(16);not null;index"`
	ErrorCode                  string         `json:"error_code,omitempty" gorm:"type:varchar(64);not null;default:''"`
	PromptTokens               *int           `json:"prompt_tokens,omitempty"`
	CompletionTokens           *int           `json:"completion_tokens,omitempty"`
	TotalTokens                *int           `json:"total_tokens,omitempty"`
	ProviderCacheStatus        string         `json:"provider_cache_status" gorm:"type:varchar(16);not null;default:'unreported'"`
	ProviderCacheReadTokens    *int           `json:"provider_cache_read_tokens,omitempty"`
	ProviderCacheWriteTokens   *int           `json:"provider_cache_write_tokens,omitempty"`
	ProviderCacheMissTokens    *int           `json:"provider_cache_miss_tokens,omitempty"`
	ApplicationCacheStatus     string         `json:"application_cache_status" gorm:"type:varchar(16);not null;default:'unavailable'"`
	PriceVersionID             *string        `json:"price_version_id,omitempty" gorm:"type:varchar(36)"`
	InputMicrounitsPerMillion  *int64         `json:"input_microunits_per_million,omitempty"`
	OutputMicrounitsPerMillion *int64         `json:"output_microunits_per_million,omitempty"`
	Currency                   string         `json:"currency,omitempty" gorm:"type:char(3);not null;default:''"`
	CostMicrounits             *int64         `json:"cost_microunits,omitempty"`
	AccountingComplete         bool           `json:"accounting_complete" gorm:"not null;default:false"`
	CreatedAt                  time.Time      `json:"created_at" gorm:"not null"`
	UpdatedAt                  time.Time      `json:"updated_at" gorm:"not null"`
	DeletedAt                  gorm.DeletedAt `json:"-" gorm:"index"`
}

func (ModelCallRecord) TableName() string { return "model_call_records" }

// ModelCallCompletion is the immutable finish payload for a started row.
type ModelCallCompletion struct {
	ID                       string
	EndedAt                  time.Time
	DurationMs               int64
	Status                   string
	ErrorCode                string
	PromptTokens             *int
	CompletionTokens         *int
	TotalTokens              *int
	ProviderCacheStatus      string
	ProviderCacheReadTokens  *int
	ProviderCacheWriteTokens *int
	ProviderCacheMissTokens  *int
	ApplicationCacheStatus   string
	CostMicrounits           *int64
	AccountingComplete       bool
}

// ModelPriceVersion is one immutable effective-dated price version.
type ModelPriceVersion struct {
	ID                         string     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID                   uint64     `json:"tenant_id" gorm:"not null;index"`
	ModelID                    string     `json:"model_id" gorm:"type:varchar(64);not null;index"`
	ValidFrom                  time.Time  `json:"valid_from" gorm:"not null;index"`
	ValidTo                    *time.Time `json:"valid_to,omitempty" gorm:"index"`
	InputMicrounitsPerMillion  int64      `json:"input_microunits_per_million" gorm:"not null"`
	OutputMicrounitsPerMillion int64      `json:"output_microunits_per_million" gorm:"not null"`
	Currency                   string     `json:"currency" gorm:"type:char(3);not null"`
	CreatedAt                  time.Time  `json:"created_at" gorm:"not null"`
}

func (ModelPriceVersion) TableName() string { return "model_price_versions" }

// ModelCallModelSnapshot is the credential-free identity stored in the ledger.
type ModelCallModelSnapshot struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Type     ModelType   `json:"type"`
	Source   ModelSource `json:"source"`
	Provider string      `json:"provider,omitempty"`
}
