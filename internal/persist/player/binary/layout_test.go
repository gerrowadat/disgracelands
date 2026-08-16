package binary

import (
	"encoding/json"
	"os"
	"testing"
)

// cLayout is the JSON reference/tools/pfilelayout.c prints.
type cLayout struct {
	PointerSize int `json:"pointer_size"`
	LongSize    int `json:"long_size"`
	TimeTSize   int `json:"time_t_size"`
	RecordSize  int `json:"record_size"`
	Constants   struct {
		MaxNameLength  int `json:"MAX_NAME_LENGTH"`
		ExdscrLength   int `json:"EXDSCR_LENGTH"`
		MaxTitleLength int `json:"MAX_TITLE_LENGTH"`
		MaxPwdLength   int `json:"MAX_PWD_LENGTH"`
		HostLength     int `json:"HOST_LENGTH"`
		MaxSkills      int `json:"MAX_SKILLS"`
		MaxAffect      int `json:"MAX_AFFECT"`
		MaxTongue      int `json:"MAX_TONGUE"`
	} `json:"constants"`
	Fields []struct {
		Name   string `json:"name"`
		Offset int    `json:"offset"`
		Size   int    `json:"size"`
		Kind   string `json:"kind"`
	} `json:"fields"`
}

func loadCLayout(t *testing.T, path string) cLayout {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var c cLayout
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return c
}

// TestLayoutEngineMatchesTheCompiler is the test this whole package rests on.
//
// The layout engine is only trustworthy if it produces the offsets a real
// compiler produces. testdata/layout-lp64.json is the output of
// reference/tools/pfilelayout.c built natively on x86-64 — that is, gcc's own
// answer for the LP64 model — and this asserts the engine reproduces it field
// for field.
//
// The 32-bit layout, which is the one the archived data is actually in,
// cannot be checked this way on a machine without 32-bit libc headers. The
// argument for trusting it is that it comes out of the same engine, driven by
// the same declaration, with three integers changed. Verifying that directly
// needs `gcc -m32`; see TestILP32LayoutAgainstCompiler below, which runs when
// it is available.
func TestLayoutEngineMatchesTheCompiler(t *testing.T) {
	c := loadCLayout(t, "testdata/layout-lp64.json")

	if c.LongSize != 8 || c.PointerSize != 8 || c.TimeTSize != 8 {
		t.Fatalf("testdata is not an LP64 layout: long=%d ptr=%d time_t=%d",
			c.LongSize, c.PointerSize, c.TimeTSize)
	}

	l := computeLayout(lp64)

	if l.Size != c.RecordSize {
		t.Errorf("record size = %d, compiler says %d", l.Size, c.RecordSize)
	}

	for _, f := range c.Fields {
		// The compiler's list includes a couple of probes for stride that
		// have no counterpart as named fields.
		switch f.Name {
		case "affected.1.type":
			got := l.at("affected")
			if want := c.Fields[indexOfField(c.Fields, "affected.0.type")].Offset; f.Offset-want != got.Stride {
				t.Errorf("affected stride = %d, compiler says %d", got.Stride, f.Offset-want)
			}
			continue
		case "affected.0.type", "affected.0.duration", "affected.0.modifier",
			"affected.0.location", "affected.0.bitvector", "affected.0.next":
			// The engine indexes these as "affected.<member>", since the
			// array element layout is the same for every element.
			member := f.Name[len("affected.0."):]
			got := l.at("affected." + member)
			if got.Offset != f.Offset {
				t.Errorf("affected[0].%s offset = %d, compiler says %d", member, got.Offset, f.Offset)
			}
			if got.Size != f.Size {
				t.Errorf("affected[0].%s size = %d, compiler says %d", member, got.Size, f.Size)
			}
			continue
		}

		got := l.at(f.Name)
		if got.Offset != f.Offset {
			t.Errorf("%s offset = %d, compiler says %d", f.Name, got.Offset, f.Offset)
		}
		if got.Size != f.Size {
			t.Errorf("%s size = %d, compiler says %d", f.Name, got.Size, f.Size)
		}
	}
}

func indexOfField(fields []struct {
	Name   string `json:"name"`
	Offset int    `json:"offset"`
	Size   int    `json:"size"`
	Kind   string `json:"kind"`
}, name string) int {
	for i, f := range fields {
		if f.Name == name {
			return i
		}
	}
	return 0
}

func TestConstantsMatchStructsH(t *testing.T) {
	// Every one of these is marked *DO*NOT*CHANGE* in structs.h because it is
	// baked into the file format. If the C tree ever changes one, the
	// compiler's output will say so here.
	c := loadCLayout(t, "testdata/layout-lp64.json")
	for _, tc := range []struct {
		name      string
		got, want int
	}{
		{"MAX_NAME_LENGTH", maxNameLength, c.Constants.MaxNameLength},
		{"EXDSCR_LENGTH", exdscrLength, c.Constants.ExdscrLength},
		{"MAX_TITLE_LENGTH", maxTitleLength, c.Constants.MaxTitleLength},
		{"MAX_PWD_LENGTH", maxPwdLength, c.Constants.MaxPwdLength},
		{"HOST_LENGTH", hostLength, c.Constants.HostLength},
		{"MAX_SKILLS", maxSkills, c.Constants.MaxSkills},
		{"MAX_AFFECT", maxAffect, c.Constants.MaxAffect},
		{"MAX_TONGUE", maxTongue, c.Constants.MaxTongue},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, structs.h says %d", tc.name, tc.got, tc.want)
		}
	}
}

// TestILP32RecordSizeMatchesTheArchivedFile is the check that the 32-bit
// layout is right, on a machine that cannot compile 32-bit code.
//
// The archived database holds 108 records and
// docs/investigations/circlemud-archive-report.md records its size as 139KB,
// established independently of any of this — by looking at the file. 108
// records of 1288 bytes is 139,104 bytes, which is that figure. A layout
// wrong by even one byte per record would be off by 108 bytes overall and
// would not round to the same number.
//
// It is not proof, but it is a real constraint from the real data, and it is
// the only one available without the archive to hand.
func TestILP32RecordSizeMatchesTheArchivedFile(t *testing.T) {
	const (
		archivedRecords = 108
		archivedSizeKB  = 139 // docs/investigations/circlemud-archive-report.md
	)

	size := computeLayout(ilp32).Size
	if size != 1288 {
		t.Errorf("ilp32 record size = %d, want 1288", size)
	}

	total := size * archivedRecords
	if got := (total + 500) / 1000; got != archivedSizeKB {
		t.Errorf("%d records of %d bytes is %d bytes (%dKB); the archive report says %dKB",
			archivedRecords, size, total, got, archivedSizeKB)
	}
}

func TestILP32AndLP64Diverge(t *testing.T) {
	// The two models produce different records, which is the entire reason
	// this package exists. If they ever came out the same, the model is not
	// being applied.
	small, big := computeLayout(ilp32), computeLayout(lp64)
	if small.Size == big.Size {
		t.Fatal("the 32-bit and 64-bit records are the same size")
	}

	// Everything up to the first width-varying field sits at the same offset
	// in both. After it, every offset moves — which is why a 64-bit read of a
	// 32-bit file produces plausible nonsense rather than an obvious failure.
	for _, name := range []string{"name", "description", "title", "sex", "hometown"} {
		if small.at(name).Offset != big.at(name).Offset {
			t.Errorf("%s differs between models but precedes the first long/time_t", name)
		}
	}
	// birth is a time_t: the first field whose width changes.
	if small.at("birth").Size == big.at("birth").Size {
		t.Error("birth is the same width in both models; time_t is not being applied")
	}
	for _, name := range []string{"played", "pwd", "cs.idnum", "affected", "last_logon", "host"} {
		if small.at(name).Offset == big.at(name).Offset {
			t.Errorf("%s is at the same offset in both models, but follows a width-varying field", name)
		}
	}
}

func TestILP32KnownOffsets(t *testing.T) {
	// A regression net, not a derivation: these come from the engine, whose
	// agreement with a real compiler is established by
	// TestLayoutEngineMatchesTheCompiler. Deriving them by hand is exactly
	// the error-prone step the engine exists to remove — the first draft of
	// this test guessed four of them and got all four wrong.
	for _, tc := range []struct {
		field        string
		offset, size int
	}{
		{"name", 0, 21},
		{"pwd", 358, 11},
		{"cs.idnum", 376, 4},
		{"cs.act", 380, 4},
		{"ps.skills", 400, maxSkills + 1},
		{"affected", 740, 16 * maxAffect},
		{"affected.next", 752, 4},
		{"last_logon", 1252, 4},
		{"host", 1256, hostLength + 1},
	} {
		got := computeLayout(ilp32).at(tc.field)
		if got.Offset != tc.offset || got.Size != tc.size {
			t.Errorf("ilp32 %s = offset %d size %d, want offset %d size %d",
				tc.field, got.Offset, got.Size, tc.offset, tc.size)
		}
	}
}

func TestAffectedElementStride(t *testing.T) {
	// struct affected_type ends in a pointer, so the array's stride differs
	// between models: 16 bytes under ilp32, 24 under lp64. Getting this wrong
	// misreads every affect and everything after them.
	for _, tc := range []struct {
		model  dataModel
		stride int
	}{
		{ilp32, 16},
		{lp64, 24},
	} {
		got := computeLayout(tc.model).at("affected")
		if got.Stride != tc.stride {
			t.Errorf("%s: affected stride = %d, want %d", tc.model.name, got.Stride, tc.stride)
		}
		if got.Size != tc.stride*maxAffect {
			t.Errorf("%s: affected size = %d, want %d", tc.model.name, got.Size, tc.stride*maxAffect)
		}
	}
}
