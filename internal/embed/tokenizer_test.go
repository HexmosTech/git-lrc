package embed

import (
	"reflect"
	"testing"
)

func testTokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	tok, err := LoadTokenizer("testdata/vocab.txt")
	if err != nil {
		t.Fatalf("LoadTokenizer: %v", err)
	}
	return tok
}

func TestLoadTokenizer(t *testing.T) {
	tok := testTokenizer(t)
	if len(tok.vocab) == 0 {
		t.Fatal("empty vocab")
	}
	if tok.clsID == 0 && tok.vocab["[CLS]"] != 0 {
		t.Errorf("clsID = %d, want vocab[[CLS]] = %d", tok.clsID, tok.vocab["[CLS]"])
	}
}

func TestEncode_ClsSep(t *testing.T) {
	tok := testTokenizer(t)
	ids := tok.Encode("orders")
	if len(ids) < 2 {
		t.Fatalf("Encode returned too few tokens: %v", ids)
	}
	if ids[0] != tok.clsID {
		t.Errorf("first token = %d, want CLS %d", ids[0], tok.clsID)
	}
	if ids[len(ids)-1] != tok.sepID {
		t.Errorf("last token = %d, want SEP %d", ids[len(ids)-1], tok.sepID)
	}
}

func TestEncode_KnownWord(t *testing.T) {
	tok := testTokenizer(t)
	// "customer" should be a whole-word vocab entry in bert-base-uncased.
	ids := tok.Encode("customer")
	want, ok := tok.vocab["customer"]
	if !ok {
		t.Fatal("vocab missing 'customer' — unexpected for bert-base-uncased vocab")
	}
	if len(ids) != 3 { // CLS, customer, SEP
		t.Fatalf("Encode(customer) = %v, want 3 tokens", ids)
	}
	if ids[1] != want {
		t.Errorf("ids[1] = %d, want %d", ids[1], want)
	}
}

func TestEncode_SubwordSplit(t *testing.T) {
	tok := testTokenizer(t)
	// A word unlikely to exist whole in the vocab but decomposable via
	// WordPiece into known subwords with a "##" continuation.
	ids := tok.Encode("dbctxifying")
	if len(ids) < 3 {
		t.Fatalf("Encode(dbctxifying) too short: %v", ids)
	}
	// Should not be a single [UNK] fallback — that would mean WordPiece
	// splitting failed to find any valid decomposition despite one existing.
	unk := tok.vocab["[UNK]"]
	allUnk := true
	for _, id := range ids[1 : len(ids)-1] {
		if id != unk {
			allUnk = false
		}
	}
	if allUnk {
		t.Errorf("Encode(dbctxifying) = %v, expected WordPiece subword split, not all-UNK", ids)
	}
}

func TestEncode_Unknown(t *testing.T) {
	tok := testTokenizer(t)
	// Emoji / symbols with no vocab coverage at all should still produce a
	// valid (UNK-containing) encoding rather than an error.
	ids := tok.Encode("\U0001F600\U0001F601")
	if len(ids) < 2 {
		t.Fatalf("Encode of emoji produced too few tokens: %v", ids)
	}
}

func TestBasicTokenize_Lowercase(t *testing.T) {
	got := basicTokenize("Orders TABLE")
	want := []string{"orders", "table"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("basicTokenize = %v, want %v", got, want)
	}
}

func TestBasicTokenize_PunctuationSplit(t *testing.T) {
	got := basicTokenize("orders.status")
	want := []string{"orders", ".", "status"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("basicTokenize = %v, want %v", got, want)
	}
}

func TestStripAccents(t *testing.T) {
	got := stripAccents("café")
	if got != "cafe" {
		t.Errorf("stripAccents(café) = %q, want cafe", got)
	}
}

func TestEncodeBatch_PadsToCommonLength(t *testing.T) {
	tok := testTokenizer(t)
	batch, attnMask, tokenType, seqLen := tok.EncodeBatch([]string{"orders", "customer purchase history"})
	if len(batch) != 2 {
		t.Fatalf("batch len = %d, want 2", len(batch))
	}
	for i, row := range batch {
		if len(row) != seqLen {
			t.Errorf("row %d len = %d, want %d", i, len(row), seqLen)
		}
		if len(attnMask[i]) != seqLen || len(tokenType[i]) != seqLen {
			t.Errorf("row %d attnMask/tokenType length mismatch", i)
		}
	}
	// Shorter sequence should have trailing PAD ids and zeroed attention mask.
	shortRow := batch[0]
	shortMask := attnMask[0]
	sawPad := false
	for i, id := range shortRow {
		if shortMask[i] == 0 {
			sawPad = true
			if id != tok.padID {
				t.Errorf("padded position %d has id %d, want PAD %d", i, id, tok.padID)
			}
		}
	}
	if !sawPad {
		t.Error("expected padding on the shorter sequence")
	}
}

func TestEncode_Truncation(t *testing.T) {
	tok := testTokenizer(t)
	long := ""
	for i := 0; i < maxTokens*2; i++ {
		long += "orders "
	}
	ids := tok.Encode(long)
	if len(ids) > maxTokens {
		t.Errorf("Encode did not truncate: len = %d, want <= %d", len(ids), maxTokens)
	}
	if ids[len(ids)-1] != tok.sepID {
		t.Error("truncated encoding should still end with SEP")
	}
}
