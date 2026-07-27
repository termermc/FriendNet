package blacklist

import (
	"context"
	pb "friendnet.org/protocol/pb/serverrpc/v1"
	"testing"
)

func mkBl() *Blacklist {
	bl, err := New(context.Background(), NewMemoryStorage())
	if err != nil {
		panic(err)
	}
	return bl
}

func TestSubstring(t *testing.T) {
	bl := mkBl()
	err := bl.AddPolicies([]*pb.BlacklistPolicy{
		{
			Keyword: "blue",
			Mode:    pb.BlacklistMatchMode_BLACKLIST_MATCH_MODE_SUBSTRING,
		},
	})
	if err != nil {
		panic(err)
	}

	if !bl.Match([]rune("thebluelight")) {
		t.Fatal("didn't match substring")
	}
}

func TestWholeAscii(t *testing.T) {
	bl := mkBl()
	err := bl.AddPolicies([]*pb.BlacklistPolicy{
		{
			Keyword: "blue",
			Mode:    pb.BlacklistMatchMode_BLACKLIST_MATCH_MODE_WHOLE,
		},
	})
	if err != nil {
		panic(err)
	}

	strs := []string{
		"out of the blue",
		"(blue)",
		"blue",
	}

	for _, str := range strs {
		if !bl.Match([]rune(str)) {
			t.Fatal("didn't match whole word in " + str)
		}
	}
}
