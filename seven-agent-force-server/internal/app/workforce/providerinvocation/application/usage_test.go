package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	invocationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/domain"
)

type recordingFinishRepository struct {
	Repository
	finish invocationdomain.Finish
}

func (r *recordingFinishRepository) Finish(_ context.Context, finish invocationdomain.Finish) error {
	r.finish = finish
	return nil
}

type interruptedProviderBody struct {
	read bool
}

func (b *interruptedProviderBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, errors.New("upstream disconnected")
	}
	b.read = true
	return copy(p, "data: {\"type\":\"message_start\"}\n\n"), nil
}

func (*interruptedProviderBody) Close() error { return nil }

func TestInterruptedStreamHasUnknownOutcome(t *testing.T) {
	repo := &recordingFinishRepository{}
	body := newObservedBody(&interruptedProviderBody{}, true, 200, "request-1", 1, repo, time.Now, func() {})
	_, _ = io.Copy(io.Discard, body)
	if repo.finish.Status != invocationdomain.StatusOutcomeUnknown || repo.finish.ErrorCode == "" {
		t.Fatalf("interrupted stream recorded as %q, code %q", repo.finish.Status, repo.finish.ErrorCode)
	}
}

func TestStreamingIdleTimeoutDoesNotLimitActiveTotalDuration(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	body := newIdleTimeoutBody(reader, 100*time.Millisecond)
	defer body.Close()
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(40 * time.Millisecond)
			_, _ = writer.Write([]byte("x"))
		}
		_ = writer.Close()
	}()
	contents, err := io.ReadAll(body)
	if err != nil || string(contents) != "xxxxx" {
		t.Fatalf("active stream interrupted: %q, %v", contents, err)
	}

	idleReader, idleWriter := io.Pipe()
	defer idleWriter.Close()
	idle := newIdleTimeoutBody(idleReader, 15*time.Millisecond)
	defer idle.Close()
	_, err = io.ReadAll(idle)
	if !errors.Is(err, errProviderStreamIdle) {
		t.Fatalf("silent stream error = %v", err)
	}
}

func TestSilentStreamAfterHeadersKeepsUnknownOutcomeAndSpecificReason(t *testing.T) {
	repo := &recordingFinishRepository{}
	reader, writer := io.Pipe()
	defer writer.Close()
	body := newObservedBody(newIdleTimeoutBody(reader, 15*time.Millisecond), true, 200, "request-2", 2, repo, time.Now, func() {})
	defer body.Close()
	_, err := io.Copy(io.Discard, body)
	if !errors.Is(err, errProviderStreamIdle) || repo.finish.Status != invocationdomain.StatusOutcomeUnknown || repo.finish.ErrorCode != "PROVIDER_STREAM_IDLE_TIMEOUT" {
		t.Fatalf("silent stream outcome=%q reason=%q error=%v", repo.finish.Status, repo.finish.ErrorCode, err)
	}
}

func value(pointer *int64) int64 {
	if pointer == nil {
		return -1
	}
	return *pointer
}

func TestParseOpenAIUsagePreservesCacheAndReasoning(t *testing.T) {
	result := parseResult(strings.NewReader(`{"model":"gpt-helper","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"cache_write_tokens":12,"output_tokens":20,"output_tokens_details":{"reasoning_tokens":8},"total_tokens":120}}`), false)
	if result.Model != "gpt-helper" || value(result.Usage.InputTokens) != 100 || value(result.Usage.CachedInputTokens) != 40 || value(result.Usage.CacheCreationInputTokens) != 12 || value(result.Usage.ReasoningOutputTokens) != 8 || value(result.Usage.TotalTokens) != 120 {
		t.Fatalf("result=%+v usage=%+v", result, result.Usage)
	}
}

func TestParseAnthropicSSEUsageMergesMessageStartAndDelta(t *testing.T) {
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-helper\",\"usage\":{\"input_tokens\":80,\"cache_read_input_tokens\":30,\"cache_creation_input_tokens\":5}}}\n\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"usage\":{\"output_tokens\":15}}}\n\n"
	result := parseResult(strings.NewReader(stream), true)
	if result.Model != "claude-helper" || value(result.Usage.InputTokens) != 80 || value(result.Usage.CachedInputTokens) != 30 || value(result.Usage.CacheCreationInputTokens) != 5 || value(result.Usage.OutputTokens) != 15 || result.Usage.TotalTokens != nil {
		t.Fatalf("result=%+v usage=%+v", result, result.Usage)
	}
}

func TestMissingUsageStaysNull(t *testing.T) {
	result := parseResult(strings.NewReader(`{"model":"gpt"}`), false)
	if result.Usage.InputTokens != nil || result.Usage.OutputTokens != nil || result.Usage.TotalTokens != nil {
		t.Fatalf("usage=%+v", result.Usage)
	}
}
