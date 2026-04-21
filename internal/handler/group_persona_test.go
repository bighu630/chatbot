package handler

import "testing"

func TestParseGroupPersona(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{
			name:  "space separated",
			input: "/persona 偏毒舌一点，但别太刻意",
			want:  "偏毒舌一点，但别太刻意",
			ok:    true,
		},
		{
			name:  "newline separated",
			input: "/persona\n偏冷淡一点",
			want:  "偏冷淡一点",
			ok:    true,
		},
		{
			name:  "missing persona",
			input: "/persona",
			want:  "",
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseGroupPersona(tt.input)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("persona = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupPersonaManagerConsumeForceNext(t *testing.T) {
	manager := NewGroupPersonaManager(nil)
	manager.forceNext[123] = true

	if !manager.ConsumeForceNext(123) {
		t.Fatal("expected first consume to return true")
	}
	if manager.ConsumeForceNext(123) {
		t.Fatal("expected second consume to return false")
	}
}

func TestValidateGroupPersona(t *testing.T) {
	if err := validateGroupPersona("正常人设"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateGroupPersona(""); err == nil {
		t.Fatal("expected empty persona to fail")
	}
	long := make([]rune, maxGroupPersonaRuneCount+1)
	for i := range long {
		long[i] = '测'
	}
	if err := validateGroupPersona(string(long)); err == nil {
		t.Fatal("expected oversized persona to fail")
	}
}
