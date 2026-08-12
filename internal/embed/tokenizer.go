package embed

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Tokenizer implements BERT-style WordPiece tokenization, matching the
// vocabulary and basic-tokenization rules used to train BGE-small-en-v1.5
// (which reuses bert-base-uncased's vocab.txt and do_lower_case=true).
// It is pure Go with no dependency on the inference backend.
type Tokenizer struct {
	vocab map[string]int64
	clsID int64
	sepID int64
	padID int64
	unkID int64
}

const (
	tokCLS = "[CLS]"
	tokSEP = "[SEP]"
	tokPAD = "[PAD]"
	tokUNK = "[UNK]"

	maxCharsPerWord = 100
)

// LoadTokenizer reads a BERT vocab.txt (one token per line, line number is
// the token ID).
func LoadTokenizer(vocabPath string) (*Tokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("open vocab: %w", err)
	}
	defer f.Close()

	vocab := make(map[string]int64)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var id int64
	for sc.Scan() {
		tok := sc.Text()
		if tok == "" {
			id++
			continue
		}
		vocab[tok] = id
		id++
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read vocab: %w", err)
	}

	t := &Tokenizer{vocab: vocab}
	var ok bool
	if t.clsID, ok = vocab[tokCLS]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokCLS)
	}
	if t.sepID, ok = vocab[tokSEP]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokSEP)
	}
	if t.padID, ok = vocab[tokPAD]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokPAD)
	}
	if t.unkID, ok = vocab[tokUNK]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokUNK)
	}
	return t, nil
}

// Encoded holds token IDs and the attention mask for a single sequence
// (already padded to a common batch length by Tokenizer.EncodeBatch).
type Encoded struct {
	IDs       []int64
	AttnMask  []int64
	TokenType []int64
}

// PadID returns the [PAD] token ID.
func (t *Tokenizer) PadID() int64 { return t.padID }

// Encode tokenizes a single string into WordPiece IDs, bracketed with
// [CLS]/[SEP] and truncated to maxTokens.
func (t *Tokenizer) Encode(text string) []int64 {
	ids := make([]int64, 0, 32)
	ids = append(ids, t.clsID)
	for _, word := range basicTokenize(text) {
		ids = append(ids, t.wordpiece(word)...)
		if len(ids) >= maxTokens-1 {
			break
		}
	}
	if len(ids) > maxTokens-1 {
		ids = ids[:maxTokens-1]
	}
	ids = append(ids, t.sepID)
	return ids
}

// EncodeBatch tokenizes multiple strings and right-pads them to a common
// length within the batch, producing input_ids/attention_mask/token_type_ids
// ready to feed to the model.
func (t *Tokenizer) EncodeBatch(texts []string) (batch [][]int64, attnMask [][]int64, tokenType [][]int64, seqLen int) {
	all := make([][]int64, len(texts))
	for i, s := range texts {
		all[i] = t.Encode(s)
		if len(all[i]) > seqLen {
			seqLen = len(all[i])
		}
	}
	batch = make([][]int64, len(texts))
	attnMask = make([][]int64, len(texts))
	tokenType = make([][]int64, len(texts))
	for i, ids := range all {
		row := make([]int64, seqLen)
		mask := make([]int64, seqLen)
		tt := make([]int64, seqLen)
		copy(row, ids)
		for j := range ids {
			mask[j] = 1
		}
		for j := len(ids); j < seqLen; j++ {
			row[j] = t.padID
		}
		batch[i], attnMask[i], tokenType[i] = row, mask, tt
	}
	return batch, attnMask, tokenType, seqLen
}

// wordpiece greedily splits a single already-lowercased, punctuation-free
// word into vocabulary subwords using the standard WordPiece
// longest-match-first algorithm, falling back to [UNK] when no split works.
func (t *Tokenizer) wordpiece(word string) []int64 {
	runes := []rune(word)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) > maxCharsPerWord {
		return []int64{t.unkID}
	}

	var out []int64
	start := 0
	for start < len(runes) {
		end := len(runes)
		found := false
		for end > start {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := t.vocab[sub]; ok {
				out = append(out, id)
				start = end
				found = true
				break
			}
			end--
		}
		if !found {
			return []int64{t.unkID}
		}
	}
	return out
}

// basicTokenize lowercases, strips accents, and splits text into words and
// individual punctuation characters, mirroring BERT's BasicTokenizer.
func basicTokenize(text string) []string {
	text = strings.ToLower(text)
	text = stripAccents(text)

	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case r == 0 || r == 0xfffd || unicode.Is(unicode.Cc, r):
			continue
		case unicode.IsSpace(r):
			flush()
		case isPunct(r):
			flush()
			tokens = append(tokens, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func isPunct(r rune) bool {
	// Matches BERT's _is_punctuation: ASCII punctuation ranges plus the
	// Unicode punctuation/symbol categories.
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// stripAccents removes combining marks after NFD decomposition, matching
// BERT's accent stripping under do_lower_case=true.
func stripAccents(s string) string {
	decomposed := norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
