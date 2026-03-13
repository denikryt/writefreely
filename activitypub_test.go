package writefreely

import (
	"testing"
	"time"

	"github.com/guregu/null/zero"
	"github.com/writeas/web-core/activitystreams"
	"github.com/writefreely/writefreely/config"
)

var actorTestTable = []struct {
	Name string
	Resp []byte
}{
	{
		"Context as a string",
		[]byte(`{"@context":"https://www.w3.org/ns/activitystreams"}`),
	},
	{
		"Context as a list",
		[]byte(`{"@context":["one string", "two strings"]}`),
	},
}

func TestUnmarshalActor(t *testing.T) {
	for _, tc := range actorTestTable {
		actor := activitystreams.Person{}
		err := unmarshalActor(tc.Resp, &actor)
		if err != nil {
			t.Errorf("%s failed with error %s", tc.Name, err)
		}
	}
}

func TestActivityObjectUsesExplicitTags(t *testing.T) {
	wasSingleUser := isSingleUser
	isSingleUser = true
	defer func() { isSingleUser = wasSingleUser }()

	cfg := config.Config{}
	cfg.App.Host = "https://example.com"
	app := &App{cfg: &cfg}

	post := &PublicPost{
		Post: &Post{
			ID:       "abc123",
			Title:    zero.StringFrom("Hello"),
			Content:  "Body without mentions or hashtags.",
			Tags:     []string{"linux", "fediverse"},
			Created:  time.Unix(1, 0).UTC(),
			Language: zero.StringFrom("en"),
		},
		Collection: &CollectionObj{Collection: Collection{
			Alias: "me",
		}},
	}
	post.Collection.hostName = "https://example.com"

	obj := post.ActivityObject(app)

	if len(obj.Tag) != 2 {
		t.Fatalf("expected 2 ActivityPub hashtag tags, got %d", len(obj.Tag))
	}
	if obj.Tag[0].Type != activitystreams.TagHashtag || obj.Tag[0].Name != "#linux" || obj.Tag[0].HRef != "https://example.com/tag:linux" {
		t.Fatalf("unexpected first AP tag: %#v", obj.Tag[0])
	}
	if obj.Tag[1].Type != activitystreams.TagHashtag || obj.Tag[1].Name != "#fediverse" || obj.Tag[1].HRef != "https://example.com/tag:fediverse" {
		t.Fatalf("unexpected second AP tag: %#v", obj.Tag[1])
	}
}

func TestActivityObjectIgnoresTextHashtags(t *testing.T) {
	wasSingleUser := isSingleUser
	isSingleUser = true
	defer func() { isSingleUser = wasSingleUser }()

	cfg := config.Config{}
	cfg.App.Host = "https://example.com"
	app := &App{cfg: &cfg}

	post := &PublicPost{
		Post: &Post{
			ID:      "abc123",
			Title:   zero.StringFrom("Hello"),
			Content: "Body with #linux in text only.",
			Created: time.Unix(1, 0).UTC(),
		},
		Collection: &CollectionObj{Collection: Collection{
			Alias: "me",
		}},
	}
	post.Collection.hostName = "https://example.com"

	obj := post.ActivityObject(app)

	for _, tag := range obj.Tag {
		if tag.Type == activitystreams.TagHashtag {
			t.Fatalf("did not expect hashtag AP tags from post body text, got %#v", obj.Tag)
		}
	}
}
