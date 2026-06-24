package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func mkLines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("line %d", i)
	}
	return out
}

// rows returns View()'s output split into visual rows.
func rows(v *logView) []string { return strings.Split(v.View(), "\n") }

func TestLogViewFollowShowsTail(t *testing.T) {
	v := &logView{follow: true}
	v.resize(20, 5)
	v.setLines(mkLines(100))

	out := v.View()
	if !strings.Contains(out, "line 99") || !strings.Contains(out, "line 95") {
		t.Fatalf("follow should show the tail (95-99):\n%s", out)
	}
	if strings.Contains(out, "line 0\n") || strings.Contains(out, "line 50") {
		t.Fatalf("follow should not show old lines:\n%s", out)
	}
}

func TestLogViewAlwaysExactHeight(t *testing.T) {
	v := &logView{follow: true}
	v.resize(20, 6)
	v.setLines(mkLines(3)) // fewer lines than height -> must pad
	if got := len(rows(v)); got != 6 {
		t.Fatalf("View must always return height rows: got %d, want 6", got)
	}
	for _, r := range rows(v) {
		if w := lipgloss.Width(r); w != 20 {
			t.Fatalf("each row must be padded to width 20, got %d (%q)", w, r)
		}
	}
}

func TestLogViewScrollUpDisablesFollowAndShowsEarlier(t *testing.T) {
	v := &logView{follow: true}
	v.resize(20, 5)
	v.setLines(mkLines(100)) // tail = lines 95..99

	v.scrollUp(3) // -> top at line 92
	if v.follow {
		t.Fatal("scrolling up must disable follow")
	}
	if out := v.View(); !strings.Contains(out, "line 92") || strings.Contains(out, "line 99") {
		t.Fatalf("scroll up should reveal earlier lines and drop the tail:\n%s", out)
	}
}

func TestLogViewGotoTopBottom(t *testing.T) {
	v := &logView{follow: true}
	v.resize(20, 5)
	v.setLines(mkLines(100))

	v.gotoTop()
	if v.follow {
		t.Fatal("gotoTop must disable follow")
	}
	if out := v.View(); !strings.Contains(out, "line 0") || strings.Contains(out, "line 99") {
		t.Fatalf("gotoTop should show the head:\n%s", out)
	}
	v.gotoBottom()
	if !v.follow {
		t.Fatal("gotoBottom must enable follow")
	}
	if out := v.View(); !strings.Contains(out, "line 99") {
		t.Fatalf("gotoBottom should show the tail:\n%s", out)
	}
}

func TestLogViewScrollClampsAtEnds(t *testing.T) {
	v := &logView{}
	v.resize(20, 5)
	v.setLines(mkLines(20))

	v.scrollUp(1000) // can't go above the top
	if !strings.Contains(v.View(), "line 0") {
		t.Fatalf("over-scroll up should clamp at the first line:\n%s", v.View())
	}
	v.scrollDown(1000) // can't go below the last full page (lines 15..19)
	out := v.View()
	if !strings.Contains(out, "line 19") || !strings.Contains(out, "line 15") {
		t.Fatalf("over-scroll down should clamp at the last page:\n%s", out)
	}
}

func TestLogViewWideLineWrapsTailAndHead(t *testing.T) {
	// A single logical line of 30 chars wraps to 3 rows at width 10; the window is
	// only 2 rows, so follow shows its last 2 rows and gotoTop its first 2.
	long := strings.Repeat("a", 10) + strings.Repeat("b", 10) + strings.Repeat("c", 10)
	v := &logView{follow: true}
	v.resize(10, 2)
	v.setLines([]string{long})

	if out := v.View(); !strings.Contains(out, "bbbbbbbbbb") || !strings.Contains(out, "cccccccccc") || strings.Contains(out, "aaaaaaaaaa") {
		t.Fatalf("follow should show the wrapped line's tail rows:\n%s", out)
	}
	v.gotoTop()
	if out := v.View(); !strings.Contains(out, "aaaaaaaaaa") || !strings.Contains(out, "bbbbbbbbbb") || strings.Contains(out, "cccccccccc") {
		t.Fatalf("gotoTop should show the wrapped line's head rows:\n%s", out)
	}
}

func TestLogViewResizeReWrapsAndKeepsTail(t *testing.T) {
	v := &logView{follow: true}
	v.resize(80, 5)
	v.setLines(mkLines(100))
	v.resize(20, 4) // narrower + shorter
	if got := len(rows(v)); got != 4 {
		t.Fatalf("after resize View must return new height: got %d, want 4", got)
	}
	if !strings.Contains(v.View(), "line 99") {
		t.Fatalf("following tail must survive a resize:\n%s", v.View())
	}
}

func TestLogViewEmpty(t *testing.T) {
	v := &logView{follow: true}
	v.resize(20, 4)
	v.setLines(nil)
	if got := len(rows(v)); got != 4 {
		t.Fatalf("empty view must still pad to height: got %d, want 4", got)
	}
	v.scrollUp(5) // must not panic
	v.scrollDown(5)
	v.gotoTop()
	v.gotoBottom()
}
