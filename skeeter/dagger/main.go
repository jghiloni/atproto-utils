// Add the ability to post to the Bluesky social network
//
// This adds the ability to post to Bluesky (aka skeets). It will parse
// URLs from post text and add links if desired, and will upload images
// as well

package main

import (
	"context"
	"dagger/skeeter/internal/dagger"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/ipfs/go-cid"
)

// The struct represents the information necessary to post to Bluesky
type Skeeter struct {
	// +private
	PDSURL string

	// +private
	Username string

	// +private
	AppPassword *dagger.Secret
}

// Creates a new Skeeter instance
func New(
	// Custom PDS URL (defaults to https://bsky.social)
	// +optional
	// +default="https://bsky.social"
	pdsURL string,
	// bsky username (without the leading @)
	// +optional
	username string,
	// bsky app password. get an app password at https://bsky.app/settings/app-passwords
	// +optional
	appPassword *dagger.Secret,
) *Skeeter {
	return &Skeeter{
		PDSURL:      pdsURL,
		Username:    username,
		AppPassword: appPassword,
	}
}

// Sets the bluesky custom Personal Data Server URL
func (s *Skeeter) WithCustomPDSURL(pdsURL string) *Skeeter {
	s.PDSURL = pdsURL
	return s
}

// Sets the bluesky username
func (s *Skeeter) WithUsername(username string) *Skeeter {
	s.Username = username
	return s
}

// Sets the app password for a bluesky user (see https://bsky.app/settings/app-passwords)
func (s *Skeeter) WithAppPassword(appPassword *dagger.Secret) *Skeeter {
	s.AppPassword = appPassword
	return s
}

// Creates (and maybe publishes) a Bluesky post, or skeet
func (s *Skeeter) Publish(
	ctx context.Context,
	// The text to post to Bluesky
	postText string,
	// If true, parse text and convert URLs to hyperlinks
	// +optional
	// +default=true
	parseLinks bool,
	// If false, do not publish the skeet. If this is false, the value returned will be the post serialized to JSON
	// +optional
	// +default=true
	publish bool,
	// Any images listed will be uploaded to bluesky and embedded
	// +optional
	images ...*dagger.File,
) (string, error) {
	post := bsky.FeedPost{
		CreatedAt: time.Now().Format(time.RFC3339),
		Text:      postText,
	}

	linkRE := regexp.MustCompile(`(?i)(?:http[s]?:\/\/.)?(?:www\.)?[-a-zA-Z0-9@%._\+~#=]{2,256}\.[a-z]{2,6}\b(?:[-a-zA-Z0-9@:%_\+.~#?&\/\/=]*)`)

	if parseLinks {
		ranges := linkRE.FindAllStringIndex(postText, -1)
		post.Facets = make([]*bsky.RichtextFacet, 0, len(ranges))
		for _, r := range ranges {
			fmt.Printf("%v\n", r)
			fmt.Println(postText[r[0]:r[1]])
			post.Facets = append(post.Facets, &bsky.RichtextFacet{
				Index: &bsky.RichtextFacet_ByteSlice{
					ByteStart: int64(r[0]),
					ByteEnd:   int64(r[1]),
				},
				Features: []*bsky.RichtextFacet_Features_Elem{
					{
						RichtextFacet_Link: &bsky.RichtextFacet_Link{
							Uri: postText[r[0]:r[1]],
						},
					},
				},
			})
		}
	}

	pw, err := s.AppPassword.Plaintext(ctx)
	if err != nil {
		return "", err
	}
	loginInput := &atproto.ServerCreateSession_Input{
		Identifier: s.Username,
		Password:   pw,
	}
	userAgent := "dagger.io/bsky-poster"
	xrpcClient := &xrpc.Client{
		Client:    util.RobustHTTPClient(),
		Host:      s.PDSURL,
		UserAgent: &userAgent,
	}
	authResult, err := atproto.ServerCreateSession(ctx, xrpcClient, loginInput)
	if err != nil {
		return "", fmt.Errorf("could not log in to Bluesky: %w", err)
	}

	xrpcClient.Auth = &xrpc.AuthInfo{
		AccessJwt:  authResult.AccessJwt,
		RefreshJwt: authResult.RefreshJwt,
		Handle:     authResult.Handle,
		Did:        authResult.Did,
	}

	embeddedImages := make([]*bsky.EmbedImages_Image, 0, len(images))
	for _, img := range images {
		contents, err := img.Contents(ctx)
		if err != nil {
			return "", err
		}

		name, err := img.Name(ctx)
		if err != nil {
			return "", err
		}

		eImg := &bsky.EmbedImages_Image{
			Alt: fmt.Sprintf("Embedded image %s", name),
		}

		eImg.Image = &lexutil.LexBlob{
			MimeType: "fake",
			Size:     0,
			Ref:      lexutil.LexLink(cid.Undef),
		}
		if publish {
			output, err := atproto.RepoUploadBlob(ctx, xrpcClient, strings.NewReader(contents))
			if err != nil {
				return "", err
			}
			eImg.Image = output.Blob
		}

		embeddedImages = append(embeddedImages, eImg)
	}

	post.Embed = &bsky.FeedPost_Embed{
		EmbedImages: &bsky.EmbedImages{
			Images: embeddedImages,
		},
	}

	if !publish {
		stringWriter := &strings.Builder{}
		encoder := json.NewEncoder(stringWriter)
		encoder.SetIndent("", "  ")
		if err = encoder.Encode(post); err != nil {
			return "", err
		}
		return stringWriter.String(), errors.New("publish disabled")
	}

	result, err := atproto.RepoCreateRecord(ctx, xrpcClient, &atproto.RepoCreateRecord_Input{
		Collection: "app.bsky.feed.post",
		Repo:       xrpcClient.Auth.Did,
		Record: &lexutil.LexiconTypeDecoder{
			Val: &post,
		},
	})

	if err != nil {
		return "", err
	}

	return result.Uri, nil
}
