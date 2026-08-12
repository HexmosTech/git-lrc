package embed

import (
	"fmt"
	"math"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// onnxRuntime input/output tensor names for the pinned BAAI/bge-small-en-v1.5
// onnx/model.onnx export. Fixed rather than introspected at load time since
// dbctx always uses the one pinned model asset.
const (
	inputIDsName      = "input_ids"
	attentionMaskName = "attention_mask"
	tokenTypeIDsName  = "token_type_ids"
	outputHiddenName  = "last_hidden_state"
)

// batchSize bounds how many texts are embedded in a single ONNX Run call.
// Larger batches trade memory for throughput; this keeps peak memory
// bounded regardless of how many schema objects are being indexed at once.
const batchSize = 16

var envOnce struct {
	sync.Once
	err error
}

// ensureEnvironment initializes the process-wide onnxruntime environment
// exactly once. onnxruntime_go's environment is a global singleton, so
// repeated OnnxEmbedder instances within one process share it.
func ensureEnvironment(libPath string) error {
	envOnce.Do(func() {
		ort.SetSharedLibraryPath(libPath)
		envOnce.err = ort.InitializeEnvironment()
	})
	return envOnce.err
}

// OnnxEmbedder runs BGE-small-en-v1.5 locally via the onnxruntime C API
// (dynamically loaded, no link-time dependency). It implements the
// semantic.Embedder interface structurally.
type OnnxEmbedder struct {
	mu   sync.Mutex
	tok  *Tokenizer
	sess *ort.DynamicAdvancedSession
}

// NewOnnxEmbedder loads the tokenizer vocabulary and opens an inference
// session against modelPath, using the onnxruntime shared library at
// libPath. Both paths are typically produced by EnsureModel and
// EnsureRuntimeLibrary.
func NewOnnxEmbedder(libPath, modelPath, vocabPath string) (*OnnxEmbedder, error) {
	if err := ensureEnvironment(libPath); err != nil {
		return nil, fmt.Errorf("initialize onnxruntime environment: %w", err)
	}

	tok, err := LoadTokenizer(vocabPath)
	if err != nil {
		return nil, err
	}

	sess, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{inputIDsName, attentionMaskName, tokenTypeIDsName},
		[]string{outputHiddenName}, nil)
	if err != nil {
		return nil, fmt.Errorf("load onnx model %s: %w", modelPath, err)
	}

	return &OnnxEmbedder{tok: tok, sess: sess}, nil
}

// Close releases the inference session. The shared onnxruntime environment
// is process-global and is not torn down (see ensureEnvironment).
func (e *OnnxEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sess != nil {
		err := e.sess.Destroy()
		e.sess = nil
		return err
	}
	return nil
}

// Dims returns the embedding dimensionality (384 for bge-small-en-v1.5).
func (e *OnnxEmbedder) Dims() int { return Dims }

// ModelID returns the stable model+backend identifier persisted alongside
// embeddings for compatibility checking.
func (e *OnnxEmbedder) ModelID() string { return ModelID }

// EmbedPassages embeds indexed schema-object text. No instruction prefix is
// added — BAAI's usage guidance is that passages/corpus text should never
// carry the retrieval instruction, only short queries should.
func (e *OnnxEmbedder) EmbedPassages(texts []string) ([][]float32, error) {
	return e.embed(texts)
}

// EmbedQuery embeds a single natural-language query, prepending BGE's
// documented retrieval instruction prefix to improve short-query recall.
func (e *OnnxEmbedder) EmbedQuery(text string) ([]float32, error) {
	vecs, err := e.embed([]string{queryInstructionPrefix + text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *OnnxEmbedder) embed(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := e.embedBatch(texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (e *OnnxEmbedder) embedBatch(texts []string) ([][]float32, error) {
	batch, attnMask, _, seqLen := e.tok.EncodeBatch(texts)
	n := len(texts)

	flatIDs := make([]int64, n*seqLen)
	flatAttn := make([]int64, n*seqLen)
	flatType := make([]int64, n*seqLen)
	for i := 0; i < n; i++ {
		copy(flatIDs[i*seqLen:(i+1)*seqLen], batch[i])
		copy(flatAttn[i*seqLen:(i+1)*seqLen], attnMask[i])
		// token_type_ids are all zero for single-sequence input; flatType
		// is already zero-valued.
	}

	shape := ort.NewShape(int64(n), int64(seqLen))
	idsT, err := ort.NewTensor(shape, flatIDs)
	if err != nil {
		return nil, fmt.Errorf("build input_ids tensor: %w", err)
	}
	defer idsT.Destroy()
	attnT, err := ort.NewTensor(shape, flatAttn)
	if err != nil {
		return nil, fmt.Errorf("build attention_mask tensor: %w", err)
	}
	defer attnT.Destroy()
	typeT, err := ort.NewTensor(shape, flatType)
	if err != nil {
		return nil, fmt.Errorf("build token_type_ids tensor: %w", err)
	}
	defer typeT.Destroy()

	outShape := ort.NewShape(int64(n), int64(seqLen), int64(Dims))
	outT, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		return nil, fmt.Errorf("allocate output tensor: %w", err)
	}
	defer outT.Destroy()

	e.mu.Lock()
	runErr := e.sess.Run([]ort.Value{idsT, attnT, typeT}, []ort.Value{outT})
	e.mu.Unlock()
	if runErr != nil {
		return nil, fmt.Errorf("onnx inference: %w", runErr)
	}

	data := outT.GetData()
	result := make([][]float32, n)
	for i := 0; i < n; i++ {
		// CLS pooling: token index 0 of each sequence, per
		// BAAI/bge-small-en-v1.5's 1_Pooling/config.json
		// (pooling_mode_cls_token: true).
		vec := make([]float32, Dims)
		off := i * seqLen * Dims
		copy(vec, data[off:off+Dims])
		normalize(vec)
		result[i] = vec
	}
	return result, nil
}

// normalize L2-normalizes vec in place, matching the sentence-transformers
// Normalize module applied after pooling for bge-*-v1.5 models.
func normalize(vec []float32) {
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return
	}
	for i, v := range vec {
		vec[i] = float32(float64(v) / norm)
	}
}
