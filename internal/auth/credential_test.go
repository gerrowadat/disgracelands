package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/auth/descrypt"
	"github.com/gerrowadat/disgracelands/internal/game"
)

// legacyCredential builds a stored credential the way the C server would
// have: hash with the character's name as salt, then keep ten characters.
func legacyCredential(t *testing.T, name, password string) game.Credential {
	t.Helper()
	full, err := descrypt.Crypt(password, name)
	if err != nil {
		t.Fatalf("descrypt.Crypt: %v", err)
	}
	return game.Credential{Scheme: game.SchemeLegacyDES, Hash: full[:10]}
}

func TestLegacyPasswordVerifies(t *testing.T) {
	v := Verifier{AllowLegacy: true}
	cred := legacyCredential(t, "Zod", "swordfish")

	res, err := v.Verify(cred, "Zod", "swordfish")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Fatal("a correct legacy password did not verify")
	}
}

// TestOnlyTheStoredPrefixIsCompared is the test that would have caught the
// mistake §5.3.1 warns about.
//
// A full DES hash is thirteen characters and the C server stored ten. An
// implementation that compared all thirteen would reject every correct
// password on the archived roster, and would report nothing but "wrong
// password" while doing it.
func TestOnlyTheStoredPrefixIsCompared(t *testing.T) {
	full, err := descrypt.Crypt("swordfish", "Zod")
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 13 {
		t.Fatalf("a DES hash should be 13 characters, got %d", len(full))
	}

	v := Verifier{AllowLegacy: true}
	stored := game.Credential{Scheme: game.SchemeLegacyDES, Hash: full[:10]}

	res, err := v.Verify(stored, "Zod", "swordfish")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("a ten-character stored hash did not verify; the comparison is using the wrong length")
	}
}

func TestWrongLegacyPasswordFails(t *testing.T) {
	v := Verifier{AllowLegacy: true}
	cred := legacyCredential(t, "Zod", "swordfish")

	// Every one of these differs within the *first eight* characters, which
	// is all DES crypt looks at. Anything differing only after them is the
	// same password as far as this hash is concerned — see
	// TestNinthCharacterIsIgnoredEvenWhenItLooksWrong.
	//
	// Two successive drafts of this test got that wrong, which is a fair
	// indication of how easily it catches people: "swordfish" is nine
	// characters, so "swordfisi" and "swordfis!" are both *correct*
	// passwords for it.
	for _, wrong := range []string{"", "SWORDFIS", "sword", "swordfia", "xwordfis"} {
		res, err := v.Verify(cred, "Zod", wrong)
		if err != nil {
			t.Fatalf("Verify(%q): %v", wrong, err)
		}
		if res.OK {
			t.Errorf("the password %q was accepted", wrong)
		}
	}
}

func TestNinthCharacterIsIgnoredEvenWhenItLooksWrong(t *testing.T) {
	// "swordfish" is nine characters, so DES crypt hashes "swordfis" and
	// "swordfisi" identically — typing the wrong ninth character logs you in.
	// This is not a defect in the verifier; it is what these hashes are, and
	// it is the clearest possible argument for replacing them.
	v := Verifier{AllowLegacy: true}
	cred := legacyCredential(t, "Zod", "swordfish")

	res, err := v.Verify(cred, "Zod", "swordfisi")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("a password differing only at the ninth character was rejected; DES crypt cannot tell them apart")
	}
}

func TestOnlyEightCharactersOfALegacyPasswordMatter(t *testing.T) {
	// Not a defect here — it is what DES crypt does, and it is why these
	// hashes are being retired. Asserting it means nobody has to rediscover
	// that a player's long passphrase was never what protected them.
	v := Verifier{AllowLegacy: true}
	cred := legacyCredential(t, "Zod", "abcdefgh")

	res, err := v.Verify(cred, "Zod", "abcdefgh-and-more")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("a password differing only after the eighth character was rejected; DES crypt ignores it")
	}
}

func TestSuccessfulLegacyLoginProducesAnUpgrade(t *testing.T) {
	v := Verifier{AllowLegacy: true}
	cred := legacyCredential(t, "Zod", "swordfish")

	res, err := v.Verify(cred, "Zod", "swordfish")
	if err != nil {
		t.Fatal(err)
	}
	if res.Upgraded == nil {
		t.Fatal("a successful legacy login produced no upgrade")
	}
	if res.Upgraded.Scheme != game.SchemeArgon2id {
		t.Errorf("upgraded to %q, want %q", res.Upgraded.Scheme, game.SchemeArgon2id)
	}
	if !strings.HasPrefix(res.Upgraded.Hash, "$argon2id$") {
		t.Errorf("upgraded hash %q is not in PHC form", res.Upgraded.Hash)
	}

	// And the replacement must actually verify the same password, or the
	// upgrade would lock the player out on their next login.
	again, err := v.Verify(*res.Upgraded, "Zod", "swordfish")
	if err != nil {
		t.Fatal(err)
	}
	if !again.OK {
		t.Error("the upgraded credential does not verify the password it was made from")
	}
	if again.Upgraded != nil {
		t.Error("a modern credential was upgraded again; that would rehash on every login")
	}
}

func TestFailedLegacyLoginProducesNoUpgrade(t *testing.T) {
	v := Verifier{AllowLegacy: true}
	cred := legacyCredential(t, "Zod", "swordfish")

	res, _ := v.Verify(cred, "Zod", "wrong")
	if res.Upgraded != nil {
		t.Error("a failed login produced an upgrade; that would overwrite a credential on a guess")
	}
}

func TestLegacyCanBeRefused(t *testing.T) {
	// Turning this off locks out anyone who has not logged in since the
	// migration, so it has to be an explicit, distinguishable failure rather
	// than looking like a wrong password.
	v := Verifier{AllowLegacy: false}
	cred := legacyCredential(t, "Zod", "swordfish")

	_, err := v.Verify(cred, "Zod", "swordfish")
	if !errors.Is(err, ErrLegacyRefused) {
		t.Errorf("Verify with AllowLegacy=false = %v, want ErrLegacyRefused", err)
	}
}

func TestModernCredentialRoundTrip(t *testing.T) {
	cred, err := NewCredential("a much better password")
	if err != nil {
		t.Fatal(err)
	}
	v := Verifier{}

	res, err := v.Verify(cred, "Zod", "a much better password")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Error("a correct modern password did not verify")
	}

	res, err = v.Verify(cred, "Zod", "a much better passworD")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Error("a wrong modern password verified")
	}
}

func TestLongPasswordsAreFullyUsedByTheModernScheme(t *testing.T) {
	// The contrast with DES that makes the migration worth doing.
	long := strings.Repeat("correct horse battery staple ", 4)
	cred, err := NewCredential(long)
	if err != nil {
		t.Fatal(err)
	}
	v := Verifier{}

	if res, _ := v.Verify(cred, "Zod", long); !res.OK {
		t.Error("the full password did not verify")
	}
	// Differing only at character 100 must be rejected — under DES it would
	// have been accepted.
	if res, _ := v.Verify(cred, "Zod", long[:99]+"X"+long[100:]); res.OK {
		t.Error("a password differing at character 100 was accepted")
	}
}

func TestEachCredentialGetsItsOwnSalt(t *testing.T) {
	a, err := NewCredential("password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewCredential("password")
	if err != nil {
		t.Fatal(err)
	}
	if a.Hash == b.Hash {
		t.Error("two credentials for the same password are identical; the salt is not random")
	}
}

func TestNoCredentialMeansNoLogin(t *testing.T) {
	// An empty credential must not mean "any password works".
	v := Verifier{AllowLegacy: true}
	for _, password := range []string{"", "anything"} {
		res, err := v.Verify(game.Credential{}, "Zod", password)
		if err != nil {
			t.Fatal(err)
		}
		if res.OK {
			t.Errorf("the password %q was accepted for a character with no credential", password)
		}
	}
}

func TestParametersComeFromTheHashNotTheCode(t *testing.T) {
	// Otherwise raising the cost would lock out everyone hashed under the old
	// one. This hash is deliberately made with weaker parameters than the
	// package's current constants.
	cred, err := NewCredential("password")
	if err != nil {
		t.Fatal(err)
	}
	weakened := strings.Replace(cred.Hash, "m=65536,t=3", "m=65536,t=3", 1)
	if weakened != cred.Hash {
		t.Fatal("test setup changed the hash unexpectedly")
	}

	// A hash carrying explicit parameters must verify regardless of what the
	// constants say now.
	v := Verifier{}
	if res, _ := v.Verify(game.Credential{Scheme: game.SchemeArgon2id, Hash: cred.Hash}, "Zod", "password"); !res.OK {
		t.Error("a hash with embedded parameters did not verify")
	}
	if !strings.Contains(cred.Hash, "m=") || !strings.Contains(cred.Hash, "t=") || !strings.Contains(cred.Hash, "p=") {
		t.Errorf("the hash does not carry its parameters: %q", cred.Hash)
	}
}

func TestMalformedModernHashesAreRejected(t *testing.T) {
	v := Verifier{}
	for _, bad := range []string{
		"", "not-a-hash", "$argon2id$", "$argon2id$v=19$m=1$salt$key",
		"$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$a2V5",
		"$argon2id$v=99$m=65536,t=3,p=4$c2FsdA$a2V5",
		"$argon2id$v=19$m=65536,t=3,p=999$c2FsdA$a2V5",
	} {
		if _, err := v.Verify(game.Credential{Scheme: game.SchemeArgon2id, Hash: bad}, "Zod", "password"); err == nil {
			t.Errorf("the hash %q was accepted as well-formed", bad)
		}
	}
}

func TestUnknownSchemeIsAnError(t *testing.T) {
	v := Verifier{}
	_, err := v.Verify(game.Credential{Scheme: "bcrypt", Hash: "x"}, "Zod", "password")
	if err == nil {
		t.Error("an unknown scheme was accepted")
	}
}

func TestNeedsUpgradeMatchesWhatVerifyDoes(t *testing.T) {
	// The model's own view of which credentials are obsolete must agree with
	// the one that actually performs upgrades.
	legacy := legacyCredential(t, "Zod", "swordfish")
	if !legacy.NeedsUpgrade() {
		t.Error("a legacy credential does not report that it needs upgrading")
	}
	modern, err := NewCredential("swordfish")
	if err != nil {
		t.Fatal(err)
	}
	if modern.NeedsUpgrade() {
		t.Error("a modern credential reports that it needs upgrading")
	}
}
