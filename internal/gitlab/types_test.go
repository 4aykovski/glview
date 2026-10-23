package gitlab

import "testing"

func TestMatchTagPipelines(t *testing.T) {
	tags := []Tag{{Name: "v1.14.2"}, {Name: "v1.14.1"}, {Name: "v1.14.0"}}
	pipes := []PipelineSummary{
		{ID: 95, Ref: "main"},
		{ID: 93, Ref: "v1.14.2"},
		{ID: 91, Ref: "v1.14.2"},
		{ID: 90, Ref: "v1.14.1"},
		{ID: 80, Ref: "v1.13.0"},
	}
	MatchTagPipelines(tags, pipes)
	if tags[0].Pipeline == nil || tags[0].Pipeline.ID != 93 {
		t.Fatalf("v1.14.2 = %+v, want the newest pipeline 93", tags[0].Pipeline)
	}
	if tags[1].Pipeline == nil || tags[1].Pipeline.ID != 90 {
		t.Fatalf("v1.14.1 = %+v", tags[1].Pipeline)
	}
	if tags[2].Pipeline != nil {
		t.Fatalf("v1.14.0 got a pipeline of another ref: %+v", tags[2].Pipeline)
	}
}
