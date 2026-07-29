package blacklist

import (
	"context"
	"testing"

	pb "friendnet.org/protocol/pb/serverrpc/v1"
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

	if !bl.Match([]rune("thebluelight"), "thebluelight") {
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
		if !bl.Match([]rune(str), str) {
			t.Fatal("didn't match whole word in " + str)
		}
	}
}

func TestWholeAsciiWithUnicode(t *testing.T) {
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
		"我的blue天",
		"（blue）",
	}

	for _, str := range strs {
		if !bl.Match([]rune(str), str) {
			t.Fatal("didn't match whole word in " + str)
		}
	}
}

func TestWholeUnicode(t *testing.T) {
	bl := mkBl()
	err := bl.AddPolicies([]*pb.BlacklistPolicy{
		{
			Keyword: "кот",
			Mode:    pb.BlacklistMatchMode_BLACKLIST_MATCH_MODE_WHOLE,
		},
	})
	if err != nil {
		panic(err)
	}

	strs := []string{
		"(кот)",
		"кот",
		"（кот）",
	}

	for _, str := range strs {
		if !bl.Match([]rune(str), str) {
			t.Fatal("didn't match whole word in " + str)
		}
	}

	if bl.Match([]rune("котёл"), "котёл") {
		t.Fatal("кот should not have matched substring of котёл")
	}
}
