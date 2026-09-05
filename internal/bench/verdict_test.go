package bench

import "testing"

func TestSimilarity(t *testing.T) {
	if got := similarity(`func main() { fmt.Println("hi") }`, `func main() { fmt.Println("hi") }`); got != 1 {
		t.Fatalf("identical = %v, want 1", got)
	}
	if got := similarity("func main", "def main"); got >= 0.9 {
		t.Fatalf("dissimilar = %v, want < 0.9", got)
	}
	if got := similarity("", ""); got != 1 {
		t.Fatalf("empty vs empty = %v, want 1", got)
	}
}

func TestRecommendLowest(t *testing.T) {
	extracts := []resExtract{
		{height: 144, text: "func a() {}"},
		{height: 360, text: `func main() { fmt.Println("hello world") }`},
		{height: 480, text: `func main() { fmt.Println("hello world") }`},
		{height: 1080, text: `func main() { fmt.Println("hello world") }`},
	}
	h, ok := recommendLowest(extracts, 0.9)
	if !ok {
		t.Fatal("expected a recommendation")
	}
	if h != 360 {
		t.Fatalf("recommended %dp, want 360p", h)
	}
}

func TestRecommendLowestNoneQualify(t *testing.T) {
	extracts := []resExtract{
		{height: 360, text: "totally different code"},
		{height: 1080, text: "func main() {}"},
	}
	if _, ok := recommendLowest(extracts, 0.9); ok {
		t.Fatal("expected no qualifying resolution")
	}
}

func TestRecommendLowestNoBaseline(t *testing.T) {
	if _, ok := recommendLowest([]resExtract{{height: 480, text: "x"}}, 0.9); ok {
		t.Fatal("expected false with no 1080p baseline")
	}
}
