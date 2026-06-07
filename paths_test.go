package meshcore

import "testing"

func TestTracePathForContactUsesTwoByteFallback(t *testing.T) {
	ct := Contact{
		Name:      "mc.kololec.cz",
		PublicKey: "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd",
	}
	path, size, flags, err := tracePathForContact(ct, 0)
	if err != nil {
		t.Fatal(err)
	}
	if size != 2 || flags != 1 {
		t.Fatalf("size=%d flags=%d", size, flags)
	}
	if len(path) != 2 || path[0] != 0x25 || path[1] != 0x25 {
		t.Fatalf("path = %x", path)
	}
}

func TestTracePathForContactUsesStoredOutPath(t *testing.T) {
	ct := Contact{
		Name:       "mc.kololec.cz",
		PublicKey:  "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd",
		HasPath:    true,
		OutPathEnc: 0x41, // 1 hop, 2-byte hashes
		OutPath:    []byte{0xaa, 0xbb, 0x25, 0x25},
	}
	path, size, flags, err := tracePathForContact(ct, 0)
	if err != nil {
		t.Fatal(err)
	}
	if size != 2 || flags != 1 {
		t.Fatalf("size=%d flags=%d", size, flags)
	}
	if len(path) != 2 || path[0] != 0xaa || path[1] != 0xbb {
		t.Fatalf("path = %x", path)
	}
}

func TestTracePathForContactUsesDirectOutPath(t *testing.T) {
	ct := Contact{
		Name:       "mc.kololec.cz",
		PublicKey:  "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd",
		HasPath:    true,
		OutPathEnc: 0x00, // zero-hop direct route
		OutPath:    make([]byte, 64),
	}
	path, size, flags, err := tracePathForContact(ct, 0)
	if err != nil {
		t.Fatal(err)
	}
	if size != 1 || flags != 0 {
		t.Fatalf("size=%d flags=%d", size, flags)
	}
	if len(path) != 1 || path[0] != 0x25 {
		t.Fatalf("path = %x", path)
	}
	plan, err := PlanTraceContact(ct, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Source != "contact_direct_path" {
		t.Fatalf("source = %q, want contact_direct_path", plan.Source)
	}
}

func TestTracePathForContactUsesDirectOutPathTwoByte(t *testing.T) {
	ct := Contact{
		Name:       "mc.kololec.cz",
		PublicKey:  "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd",
		HasPath:    true,
		OutPathEnc: 0x40, // zero-hop direct route, 2-byte hash size
		OutPath:    make([]byte, 64),
	}
	path, size, flags, err := tracePathForContact(ct, 0)
	if err != nil {
		t.Fatal(err)
	}
	if size != 2 || flags != 1 {
		t.Fatalf("size=%d flags=%d", size, flags)
	}
	if len(path) != 2 || path[0] != 0x25 || path[1] != 0x25 {
		t.Fatalf("path = %x", path)
	}
}

func TestPlanTraceContactExplicitPath(t *testing.T) {
	ct := Contact{
		Name:      "mc.kololec.cz",
		PublicKey: "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd",
	}
	plan, err := PlanTraceContact(ct, 3)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Source != "contact_key_fallback" || plan.HashSize != 4 || plan.HopCount != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestTracePathForContactUsesFourByteFallbackForThreeBytePrefix(t *testing.T) {
	ct := Contact{
		Name:      "mc.kololec.cz",
		PublicKey: "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd",
	}
	path, size, flags, err := tracePathForContact(ct, 3)
	if err != nil {
		t.Fatal(err)
	}
	if size != 4 || flags != 2 {
		t.Fatalf("size=%d flags=%d", size, flags)
	}
	if len(path) != 4 || path[0] != 0x25 || path[1] != 0x25 || path[2] != 0x25 || path[3] != 0xce {
		t.Fatalf("path = %x", path)
	}
}
