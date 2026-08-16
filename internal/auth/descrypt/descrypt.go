// Package descrypt implements traditional DES crypt(3), the password hash the
// Disgracelands roster was created with between 2001 and 2008.
//
// It exists to verify existing passwords once, at login, so the hash can be
// replaced with a modern one. Nothing here should ever be used to create a
// credential: DES crypt uses only the first eight characters of a password,
// its salt is twelve bits, and a single DES round-trip on modern hardware is
// trivially brute-forced. See docs/proposals/go-port-plan.md §5.3.1.
//
// # Why this is implemented rather than imported
//
// The Go standard library has no crypt(3), and crypto/des cannot be used as
// a black box for it: the salt perturbs the E expansion *inside* the round
// function, so the cipher itself is modified rather than merely being driven
// differently. The alternatives were a third-party dependency for an
// algorithm the project is actively trying to retire, or cgo — which is ruled
// out by the CGO_ENABLED=0 static build the container needs.
//
// Correctness is not argued, it is checked: descrypt_c_test.go compares this
// implementation against the system libcrypt over thousands of inputs, and
// that comparison runs in CI.
//
// The implementation favours obviousness over speed. It permutes bit by bit
// through tables that can be read against the published ones, which is
// several times slower than a table-driven implementation and entirely
// irrelevant at a few logins per second.
package descrypt

import (
	"errors"
	"strings"
)

// Alphabet is crypt(3)'s base-64 encoding, which is its own and matches
// neither standard base64 nor anything else.
const Alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// HashLength is the length of a complete traditional DES hash: two salt
// characters and eleven of digest.
const HashLength = 13

// MaxPasswordLength is how much of a password DES crypt actually uses.
// Anything after the eighth character has no effect whatsoever.
const MaxPasswordLength = 8

// ErrInvalidSalt is returned for a salt that is not two characters of the
// crypt alphabet.
var ErrInvalidSalt = errors.New("descrypt: salt must be two characters from the crypt alphabet")

// Crypt returns the traditional DES hash of password under salt.
//
// salt must be at least two characters; only the first two are used, which is
// how the C code can pass a whole stored hash as the salt and get the same
// answer as when it passed the character's name.
func Crypt(password, salt string) (string, error) {
	if len(salt) < 2 {
		return "", ErrInvalidSalt
	}
	s0, s1 := strings.IndexByte(Alphabet, salt[0]), strings.IndexByte(Alphabet, salt[1])
	if s0 < 0 || s1 < 0 {
		return "", ErrInvalidSalt
	}
	saltBits := uint(s0) | uint(s1)<<6

	// The key is the first eight characters, each shifted up one bit so the
	// low bit of every byte is the (ignored) DES parity bit.
	var key [64]byte
	for i := 0; i < MaxPasswordLength && i < len(password); i++ {
		c := int(password[i]&0x7f) << 1
		for b := 0; b < 8; b++ {
			key[i*8+b] = byte((c >> (7 - b)) & 1)
		}
	}

	subkeys := schedule(&key)
	e := expansionFor(saltBits)

	// Twenty-five encryptions of a zero block under the same key. The
	// iteration count is the format's only work factor, and it is 25.
	var block [64]byte
	for i := 0; i < 25; i++ {
		block = encrypt(block, subkeys, e)
	}

	var out strings.Builder
	out.WriteByte(salt[0])
	out.WriteByte(salt[1])
	// Sixty-four bits encoded six at a time into eleven characters; the last
	// character carries four real bits and two of padding.
	for i := 0; i < 11; i++ {
		v := 0
		for j := 0; j < 6; j++ {
			v <<= 1
			if bit := i*6 + j; bit < 64 {
				v |= int(block[bit])
			}
		}
		out.WriteByte(Alphabet[v])
	}
	return out.String(), nil
}

// expansionFor returns the E table with the salt's swaps applied.
//
// This is the whole of what makes DES crypt not DES: for each of the twelve
// salt bits that is set, entries i and i+24 of the expansion are exchanged.
// Two passwords hashed under different salts therefore run through different
// ciphers, which is what stopped off-the-shelf DES hardware being usable
// against a password file in 1979.
func expansionFor(saltBits uint) [48]byte {
	e := expansionTable
	for i := 0; i < 12; i++ {
		if saltBits&(1<<uint(i)) != 0 {
			e[i], e[i+24] = e[i+24], e[i]
		}
	}
	return e
}

// encrypt runs the sixteen DES rounds over a 64-bit block of bits.
func encrypt(block [64]byte, subkeys [16][48]byte, e [48]byte) [64]byte {
	permuted := permute(block[:], initialPermutation[:])

	var l, r [32]byte
	copy(l[:], permuted[:32])
	copy(r[:], permuted[32:])

	for round := 0; round < 16; round++ {
		next := feistel(r, subkeys[round], e)
		for i := range next {
			next[i] ^= l[i]
		}
		l, r = r, next
	}

	// The halves are exchanged once more before the final permutation, which
	// is what makes decryption the same algorithm with the subkeys reversed.
	var preoutput [64]byte
	copy(preoutput[:32], r[:])
	copy(preoutput[32:], l[:])

	final := permute(preoutput[:], finalPermutation[:])
	var out [64]byte
	copy(out[:], final)
	return out
}

// feistel is DES's round function: expand, mix in the subkey, substitute
// through the S-boxes, permute.
func feistel(r [32]byte, subkey [48]byte, e [48]byte) [32]byte {
	var expanded [48]byte
	for i, from := range e {
		expanded[i] = r[from-1] ^ subkey[i]
	}

	var substituted [32]byte
	for box := 0; box < 8; box++ {
		in := expanded[box*6 : box*6+6]
		// The outer bits pick the row, the inner four the column — the one
		// piece of DES that reads as arbitrary and is not.
		row := int(in[0])<<1 | int(in[5])
		col := int(in[1])<<3 | int(in[2])<<2 | int(in[3])<<1 | int(in[4])
		v := sBoxes[box][row*16+col]
		for b := 0; b < 4; b++ {
			substituted[box*4+b] = (v >> (3 - uint(b))) & 1
		}
	}

	var out [32]byte
	for i, from := range permutationP {
		out[i] = substituted[from-1]
	}
	return out
}

// schedule derives the sixteen round subkeys.
func schedule(key *[64]byte) [16][48]byte {
	permuted := permute(key[:], pc1[:])

	var c, d [28]byte
	copy(c[:], permuted[:28])
	copy(d[:], permuted[28:])

	var subkeys [16][48]byte
	for round := 0; round < 16; round++ {
		for s := byte(0); s < keyShifts[round]; s++ {
			c = rotate28(c)
			d = rotate28(d)
		}
		var cd [56]byte
		copy(cd[:28], c[:])
		copy(cd[28:], d[:])
		for i, from := range pc2 {
			subkeys[round][i] = cd[from-1]
		}
	}
	return subkeys
}

func rotate28(v [28]byte) [28]byte {
	first := v[0]
	copy(v[:27], v[1:])
	v[27] = first
	return v
}

// permute applies a 1-indexed permutation table, the form every published DES
// table is written in.
func permute(in []byte, table []byte) []byte {
	out := make([]byte, len(table))
	for i, from := range table {
		out[i] = in[from-1]
	}
	return out
}
