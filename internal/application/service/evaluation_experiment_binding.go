package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Tencent/WeKnora/internal/types"
)

// applyEvaluationConfigurationOverrides applies creation-request overrides to
// the resolved parameters before the experiment manifest is frozen.
func applyEvaluationConfigurationOverrides(
	params *types.ChatManage,
	overrides *types.EvaluationConfigurationOverrides,
) error {
	if overrides == nil {
		return nil
	}
	if overrides.Retrieval != nil {
		params.VectorThreshold = overrides.Retrieval.VectorThreshold
		params.KeywordThreshold = overrides.Retrieval.KeywordThreshold
		params.EmbeddingTopK = overrides.Retrieval.EmbeddingTopK
	}
	if overrides.Rerank != nil {
		params.RerankTopK = overrides.Rerank.RerankTopK
		params.RerankThreshold = overrides.Rerank.RerankThreshold
	}
	if overrides.Generation != nil {
		if overrides.Generation.Temperature != nil {
			params.SummaryConfig.Temperature = *overrides.Generation.Temperature
		}
		if overrides.Generation.TopP != nil {
			params.SummaryConfig.TopP = *overrides.Generation.TopP
		}
		if overrides.Generation.TopK != nil {
			params.SummaryConfig.TopK = *overrides.Generation.TopK
		}
		if overrides.Generation.MaxTokens != nil {
			params.SummaryConfig.MaxTokens = *overrides.Generation.MaxTokens
		}
	}
	return nil
}

// buildExperimentForTask resolves the dataset binding and model snapshots,
// then freezes the immutable experiment manifest. A nil registry keeps the
// legacy no-snapshot path for development fixtures; an explicit
// dataset_version_id without a registry is an error.
func (e *EvaluationService) buildExperimentForTask(
	ctx context.Context,
	tenantID uint64,
	options *types.EvaluationOptions,
	detail *types.EvaluationDetail,
	temporaryKBID string,
) (*types.EvaluationExperimentSnapshot, string, error) {
	if e.datasetRegistry == nil {
		if options.DatasetVersionID != "" {
			return nil, "", errors.New("build evaluation experiment: dataset registry is unavailable")
		}
		return nil, "", nil
	}

	datasetSnapshot, err := e.resolveExperimentDatasetBinding(ctx, tenantID, options.DatasetVersionID, detail.Task.DatasetID)
	if err != nil {
		return nil, "", err
	}

	// Resolve the four model roles: embedding and summary come from the
	// temporary evaluation KB configuration; chat and rerank come from the
	// resolved task parameters.
	kb, err := e.knowledgeBaseService.GetKnowledgeBaseByID(ctx, temporaryKBID)
	if err != nil {
		return nil, "", fmt.Errorf("build evaluation experiment: load evaluation knowledge base: %w", err)
	}
	modelFor := func(role, id string) (*types.Model, error) {
		if id == "" {
			return nil, nil
		}
		model, err := e.modelService.GetModelByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("build evaluation experiment: %s model %s: %w", role, id, err)
		}
		return model, nil
	}
	embeddingModel, err := modelFor("embedding", kb.EmbeddingModelID)
	if err != nil {
		return nil, "", err
	}
	summaryModel, err := modelFor("summary", kb.SummaryModelID)
	if err != nil {
		return nil, "", err
	}
	chatModel, err := modelFor("chat", detail.Params.ChatModelID)
	if err != nil {
		return nil, "", err
	}
	rerankModel, err := modelFor("rerank", detail.Params.RerankModelID)
	if err != nil {
		return nil, "", err
	}

	dbDriver := os.Getenv("DB_DRIVER")
	if dbDriver == "" {
		dbDriver = "postgres"
	}
	return BuildEvaluationExperimentSnapshot(&EvaluationExperimentInput{
		Dataset:               datasetSnapshot,
		SourceKnowledgeBaseID: options.KnowledgeBaseID,
		EmbeddingModel:        embeddingModel,
		ChatModel:             chatModel,
		RerankModel:           rerankModel,
		SummaryModel:          summaryModel,
		Params:                detail.Params,
		SeedProvided:          options.Seed != nil,
		SeedSupport:           types.EvaluationSeedSupportUnavailable,
		DBDriver:              dbDriver,
	})
}

// resolveExperimentDatasetBinding pins the dataset version for one task. An
// explicit dataset_version_id wins; otherwise the current version of the
// named dataset (usually the built-in "default") is used. Unknown ids fail
// explicitly and never fall back to a silent default.
func (e *EvaluationService) resolveExperimentDatasetBinding(
	ctx context.Context,
	tenantID uint64,
	datasetVersionID string,
	datasetID string,
) (*types.EvaluationDatasetSnapshot, error) {
	if datasetVersionID != "" {
		version, err := e.datasetRegistry.GetVersion(ctx, tenantID, datasetVersionID)
		if err != nil {
			return nil, fmt.Errorf("bind evaluation dataset version %s: %w", datasetVersionID, err)
		}
		return &types.EvaluationDatasetSnapshot{
			DatasetID:        version.DatasetID,
			DatasetVersionID: version.ID,
			VersionNumber:    version.VersionNumber,
			ArtifactSHA256:   version.ArtifactSHA256,
			ContentSHA256:    version.ContentSHA256,
		}, nil
	}

	dataset, err := e.datasetRegistry.GetDataset(ctx, tenantID, datasetID)
	if err != nil {
		return nil, fmt.Errorf("bind evaluation dataset %s: %w", datasetID, err)
	}
	if dataset.CurrentVersionID == "" {
		return nil, fmt.Errorf("bind evaluation dataset %s: no registered version", datasetID)
	}
	version, err := e.datasetRegistry.GetVersion(ctx, tenantID, dataset.CurrentVersionID)
	if err != nil {
		return nil, fmt.Errorf("bind evaluation dataset %s version %s: %w",
			datasetID, dataset.CurrentVersionID, err)
	}
	return &types.EvaluationDatasetSnapshot{
		DatasetID:        dataset.ID,
		DatasetVersionID: version.ID,
		VersionNumber:    version.VersionNumber,
		ArtifactSHA256:   version.ArtifactSHA256,
		ContentSHA256:    version.ContentSHA256,
	}, nil
}

// verifyExperimentModelFingerprints re-fetches the frozen models at run time
// and fails the task when any behavior fingerprint drifted, so one Success
// task never mixes two model configurations.
func (e *EvaluationService) verifyExperimentModelFingerprints(
	ctx context.Context,
	experiment *types.EvaluationExperimentSnapshot,
) error {
	if experiment == nil {
		return nil
	}
	roles := []struct {
		name    string
		frozen  *types.EvaluationModelSnapshot
	}{
		{"embedding", experiment.Models.Embedding},
		{"chat", experiment.Models.Chat},
		{"rerank", experiment.Models.Rerank},
		{"summary", experiment.Models.Summary},
	}
	for _, role := range roles {
		if role.frozen == nil {
			continue
		}
		current, err := e.modelService.GetModelByID(ctx, role.frozen.ID)
		if err != nil {
			return fmt.Errorf("verify evaluation %s model %s: %w", role.name, role.frozen.ID, err)
		}
		if err := VerifyEvaluationModelFingerprint(ctx, role.name, role.frozen, current); err != nil {
			return err
		}
	}
	return nil
}

// persistEvaluationExperiment fills the frozen manifest columns of one new
// task entity. The manifest is written once at creation; lifecycle
// compare-and-swap updates never touch these columns.
func persistEvaluationExperiment(
	entity *types.EvaluationTaskEntity,
	experiment *types.EvaluationExperimentSnapshot,
	experimentHash string,
) error {
	if experiment == nil {
		return nil
	}
	if len(experimentHash) != 64 {
		return errors.New("persist evaluation experiment: canonical hash is required")
	}
	encoded, err := encodeEvaluationExperiment(experiment)
	if err != nil {
		return err
	}
	datasetVersionID := experiment.Dataset.DatasetVersionID
	contentSHA256 := experiment.Dataset.ContentSHA256
	entity.DatasetVersionID = &datasetVersionID
	entity.DatasetContentSHA256 = &contentSHA256
	entity.ExperimentSnapshot = encoded
	entity.ExperimentSHA256 = &experimentHash
	return nil
}
