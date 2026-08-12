// Package embed provides local, CGO-based inference for the BGE-small-en-v1.5
// sentence embedding model via the ONNX Runtime, plus the machinery to
// download and cache the model weights and the onnxruntime shared library
// on first use. Nothing in this package is imported by dbctx's lexical
// search path — it is only pulled in when semantic indexing/search is
// actually requested, so builds that never touch semantic features never
// pay for a model download or a running inference session.
package embed

import (
	"fmt"
	"runtime"
)

// ModelID identifies the embedding model + inference backend combination
// used to produce a set of vectors. It is persisted in the .dtx metadata
// table and compared against the currently configured model before trusting
// stored embeddings — mismatches are rejected cleanly rather than causing
// silent corruption (see internal/semantic).
const ModelID = "bge-small-en-v1.5/onnx-fp32-cls-v1"

// Dims is the embedding dimensionality produced by ModelID.
const Dims = 384

// queryInstructionPrefix is prepended to queries (but never to indexed
// passages/objects) per BAAI's documented usage for bge-*-v1.5 retrieval:
// https://huggingface.co/BAAI/bge-small-en-v1.5#usage
const queryInstructionPrefix = "Represent this sentence for searching relevant passages: "

// maxTokens bounds tokenized sequence length. BGE supports up to 512
// positions, but dbctx's schema-derived text blurbs are short; capping
// lower keeps inference fast and memory-bounded.
const maxTokens = 256

// hfModelBase is the HuggingFace resolve URL for BAAI/bge-small-en-v1.5.
const hfModelBase = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main"

// modelAsset describes a single file to download and verify.
type modelAsset struct {
	Name   string // cache filename
	URL    string
	SHA256 string
	Size   int64
}

var modelWeights = modelAsset{
	Name:   "model.onnx",
	URL:    hfModelBase + "/onnx/model.onnx",
	SHA256: "828e1496d7fabb79cfa4dcd84fa38625c0d3d21da474a00f08db0f559940cf35",
	Size:   133093490,
}

var modelVocab = modelAsset{
	Name:   "vocab.txt",
	URL:    hfModelBase + "/vocab.txt",
	SHA256: "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3",
	Size:   231508,
}

// onnxRuntimeVersion is the pinned onnxruntime release. onnxruntime_go
// requires ORT API version 28+; 1.29.0 is confirmed compatible.
const onnxRuntimeVersion = "1.29.0"

// archiveKind identifies how to unpack a runtimeAsset archive.
type archiveKind int

const (
	archiveTarGz archiveKind = iota
	archiveZip
)

// runtimeAsset describes the platform-specific onnxruntime release archive
// and where the shared library lives inside it.
type runtimeAsset struct {
	URL        string
	SHA256     string
	Size       int64
	Kind       archiveKind
	MemberPath string // path of the shared library within the archive
	LocalName  string // filename to store the extracted library under
}

const ortRelBase = "https://github.com/microsoft/onnxruntime/releases/download/v" + onnxRuntimeVersion + "/"

var runtimeAssets = map[string]runtimeAsset{
	"linux/amd64": {
		URL:        ortRelBase + "onnxruntime-linux-x64-1.29.0.tgz",
		SHA256:     "c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba",
		Size:       11082880,
		Kind:       archiveTarGz,
		MemberPath: "onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so.1.29.0",
		LocalName:  "libonnxruntime.so",
	},
	"linux/arm64": {
		URL:        ortRelBase + "onnxruntime-linux-aarch64-1.29.0.tgz",
		SHA256:     "e1799098ebc054b370f6176a450f158720f297818c613e5dc99b92e2ec82346f",
		Size:       10027600,
		Kind:       archiveTarGz,
		MemberPath: "onnxruntime-linux-aarch64-1.29.0/lib/libonnxruntime.so.1.29.0",
		LocalName:  "libonnxruntime.so",
	},
	"darwin/arm64": {
		URL:        ortRelBase + "onnxruntime-osx-arm64-1.29.0.tgz",
		SHA256:     "d0706fc34f315d8c88639d0a8c81f2e09e815f282cabed3493c06a054352cf92",
		Size:       41578864,
		Kind:       archiveTarGz,
		MemberPath: "onnxruntime-osx-arm64-1.29.0/lib/libonnxruntime.1.29.0.dylib",
		LocalName:  "libonnxruntime.dylib",
	},
	"windows/amd64": {
		URL:        ortRelBase + "onnxruntime-win-x64-1.29.0.zip",
		SHA256:     "c9b4b7086b529ad814f428c1bad028e20a25d7dc0699836775faace4ab5b78b2",
		Size:       79645520,
		Kind:       archiveZip,
		MemberPath: "onnxruntime-win-x64-1.29.0/lib/onnxruntime.dll",
		LocalName:  "onnxruntime.dll",
	},
	"windows/arm64": {
		URL:        ortRelBase + "onnxruntime-win-arm64-1.29.0.zip",
		SHA256:     "a094a49c3ced0f9fca554647cc7566ae99d93a63a8ce6bf47975561c2de7608e",
		Size:       81679033,
		Kind:       archiveZip,
		MemberPath: "onnxruntime-win-arm64-1.29.0/lib/onnxruntime.dll",
		LocalName:  "onnxruntime.dll",
	},
}

// currentPlatformRuntimeAsset returns the pinned onnxruntime asset for the
// running GOOS/GOARCH, or an error naming platforms that have no published
// build (notably darwin/amd64 — Intel Macs — which onnxruntime dropped from
// its 1.29.0 release). Callers on unsupported platforms should point
// DBCTX_ONNXRUNTIME_LIB at a self-supplied onnxruntime shared library.
func currentPlatformRuntimeAsset() (runtimeAsset, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	a, ok := runtimeAssets[key]
	if !ok {
		return runtimeAsset{}, fmt.Errorf(
			"no prebuilt onnxruntime %s available for %s; set DBCTX_ONNXRUNTIME_LIB to a local onnxruntime shared library to enable semantic search on this platform",
			onnxRuntimeVersion, key)
	}
	return a, nil
}
