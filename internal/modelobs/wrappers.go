package modelobs

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
)

type observedChat struct {
	recorder *Recorder
	model    *types.Model
	inner    chat.Chat
}

func (r *Recorder) WrapChat(model *types.Model, inner chat.Chat) chat.Chat {
	if r == nil || r.store == nil || inner == nil {
		return inner
	}
	return &observedChat{recorder: r, model: model, inner: inner}
}

func (o *observedChat) Chat(ctx context.Context, messages []chat.Message, opts *chat.ChatOptions) (*types.ChatResponse, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "chat")
	if err != nil && strict {
		return nil, err
	}
	response, providerErr := o.inner.Chat(ctx, messages, opts)
	var usage *types.TokenUsage
	if response != nil {
		usage = &response.Usage
	}
	call.finish(ctx, statusForError(providerErr), providerErr, usage)
	return response, providerErr
}

func (o *observedChat) ChatStream(ctx context.Context, messages []chat.Message, opts *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "chat_stream")
	if err != nil && strict {
		return nil, err
	}
	upstream, providerErr := o.inner.ChatStream(ctx, messages, opts)
	if providerErr != nil {
		call.finish(ctx, statusForError(providerErr), providerErr, nil)
		return nil, providerErr
	}
	output := make(chan types.StreamResponse)
	go func() {
		defer close(output)
		status := types.ModelCallStatusSuccess
		var terminalErr error
		var usage *types.TokenUsage
		defer func() { call.finish(ctx, status, terminalErr, usage) }()
		for {
			select {
			case <-ctx.Done():
				status = types.ModelCallStatusCanceled
				terminalErr = context.Cause(ctx)
				return
			case response, ok := <-upstream:
				if !ok {
					return
				}
				if response.Usage != nil {
					copy := *response.Usage
					usage = &copy
				}
				if response.ResponseType == types.ResponseTypeError {
					status = types.ModelCallStatusError
					terminalErr = errors.New("provider stream error")
				}
				select {
				case output <- response:
				case <-ctx.Done():
					status = types.ModelCallStatusCanceled
					terminalErr = context.Cause(ctx)
					return
				}
			}
		}
	}()
	return output, nil
}

func (o *observedChat) GetModelName() string { return o.inner.GetModelName() }
func (o *observedChat) GetModelID() string   { return o.inner.GetModelID() }

type observedEmbedder struct {
	recorder *Recorder
	model    *types.Model
	inner    embedding.Embedder
}

func (r *Recorder) WrapEmbedder(model *types.Model, inner embedding.Embedder) embedding.Embedder {
	if r == nil || r.store == nil || inner == nil {
		return inner
	}
	return &observedEmbedder{recorder: r, model: model, inner: inner}
}

func (o *observedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "embedding")
	if err != nil && strict {
		return nil, err
	}
	result, providerErr := o.inner.Embed(ctx, text)
	call.finish(ctx, statusForError(providerErr), providerErr, nil)
	return result, providerErr
}
func (o *observedEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "embedding_batch")
	if err != nil && strict {
		return nil, err
	}
	result, providerErr := o.inner.BatchEmbed(ctx, texts)
	call.finish(ctx, statusForError(providerErr), providerErr, nil)
	return result, providerErr
}
func (o *observedEmbedder) BatchEmbedWithPool(ctx context.Context, _ embedding.Embedder, texts []string) ([][]float32, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "embedding_batch")
	if err != nil && strict {
		return nil, err
	}
	result, providerErr := o.inner.BatchEmbedWithPool(ctx, o.inner, texts)
	call.finish(ctx, statusForError(providerErr), providerErr, nil)
	return result, providerErr
}
func (o *observedEmbedder) GetModelName() string { return o.inner.GetModelName() }
func (o *observedEmbedder) GetDimensions() int   { return o.inner.GetDimensions() }
func (o *observedEmbedder) GetModelID() string   { return o.inner.GetModelID() }

type observedReranker struct {
	recorder *Recorder
	model    *types.Model
	inner    rerank.Reranker
}

func (r *Recorder) WrapReranker(model *types.Model, inner rerank.Reranker) rerank.Reranker {
	if r == nil || r.store == nil || inner == nil {
		return inner
	}
	return &observedReranker{recorder: r, model: model, inner: inner}
}
func (o *observedReranker) Rerank(ctx context.Context, query string, documents []string) ([]rerank.RankResult, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "rerank")
	if err != nil && strict {
		return nil, err
	}
	result, providerErr := o.inner.Rerank(ctx, query, documents)
	call.finish(ctx, statusForError(providerErr), providerErr, nil)
	return result, providerErr
}
func (o *observedReranker) GetModelName() string { return o.inner.GetModelName() }
func (o *observedReranker) GetModelID() string   { return o.inner.GetModelID() }

type observedVLM struct {
	recorder *Recorder
	model    *types.Model
	inner    vlm.VLM
}

func (r *Recorder) WrapVLM(model *types.Model, inner vlm.VLM) vlm.VLM {
	if r == nil || r.store == nil || inner == nil {
		return inner
	}
	return &observedVLM{recorder: r, model: model, inner: inner}
}
func (o *observedVLM) Predict(ctx context.Context, images [][]byte, prompt string) (string, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "vlm")
	if err != nil && strict {
		return "", err
	}
	result, providerErr := o.inner.Predict(ctx, images, prompt)
	call.finish(ctx, statusForError(providerErr), providerErr, nil)
	return result, providerErr
}
func (o *observedVLM) GetModelName() string { return o.inner.GetModelName() }
func (o *observedVLM) GetModelID() string   { return o.inner.GetModelID() }

type observedASR struct {
	recorder *Recorder
	model    *types.Model
	inner    asr.ASR
}

func (r *Recorder) WrapASR(model *types.Model, inner asr.ASR) asr.ASR {
	if r == nil || r.store == nil || inner == nil {
		return inner
	}
	return &observedASR{recorder: r, model: model, inner: inner}
}
func (o *observedASR) Transcribe(ctx context.Context, audio []byte, fileName string) (*asr.TranscriptionResult, error) {
	call, strict, err := o.recorder.start(ctx, o.model, "asr")
	if err != nil && strict {
		return nil, err
	}
	result, providerErr := o.inner.Transcribe(ctx, audio, fileName)
	call.finish(ctx, statusForError(providerErr), providerErr, nil)
	return result, providerErr
}
func (o *observedASR) GetModelName() string { return o.inner.GetModelName() }
func (o *observedASR) GetModelID() string   { return o.inner.GetModelID() }
