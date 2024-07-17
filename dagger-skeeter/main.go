// A generated module for DaggerBskyPoster functions
//
// This module has been generated via dagger init and serves as a reference to
// basic module structure as you get started with Dagger.
//
// Two functions have been pre-created. You can modify, delete, or add to them,
// as needed. They demonstrate usage of arguments and return types using simple
// echo and grep commands. The functions can be called from the dagger CLI or
// from one of the SDKs.
//
// The first line in this comment block is a short description line and the
// rest is a long description with more detail on the module's purpose or usage,
// if appropriate. All modules should have a short description.

package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jghiloni/atproto-utils/dagger-skeeter/internal/dagger"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/ipfs/go-cid"
)

// Skeeter dagger module
type Skeeter struct {
	// +private
	PDSURL string

	// +private
	Username string

	// +private
	AppPassword *dagger.Secret
}

type Skeet struct {
	x   *xrpc.Client
	p   *bsky.FeedPost
	Err error
}

var linkRE = regexp.MustCompile(`(?i)(https?://[^/]+(?::\d+)?(?:/\S+)?)`)

func New(
	// Custom PDS URL (defaults to https://bsky.social)
	// +optional
	// +default="https://bsky.social"
	pdsURL string,
	// bsky username
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

func (s *Skeeter) WithCustomPDSURL(pdsURL string) *Skeeter {
	s.PDSURL = pdsURL
	return s
}

func (s *Skeeter) WithUsername(username string) *Skeeter {
	s.Username = username
	return s
}

func (s *Skeeter) WithAppPassword(appPassword *dagger.Secret) *Skeeter {
	s.AppPassword = appPassword
	return s
}

// Create, but do not publish, a Bluesky post (skeet). It does not upload
// images
func (s *Skeeter) CreateSkeet(
	ctx context.Context,
	postText string,
	// +optional
	parseLinks bool,
	// +optional
	uploadImages bool,
	// +optional
	images ...*dagger.File,
) *Skeet {
	post := bsky.FeedPost{
		CreatedAt: time.Now().Format(time.RFC3339),
		Text:      postText,
	}

	if parseLinks {
		ranges := linkRE.FindAllStringIndex(postText, -1)
		post.Facets = make([]*bsky.RichtextFacet, 0, len(ranges))
		for _, r := range ranges {
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
		return &Skeet{Err: err}
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
		return &Skeet{Err: fmt.Errorf("could not log in to Bluesky: %w", err)}
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
			return &Skeet{Err: err}
		}

		name, err := img.Name(ctx)
		if err != nil {
			return &Skeet{Err: err}
		}

		eImg := &bsky.EmbedImages_Image{
			Alt: fmt.Sprintf("Embedded image %s", name),
		}

		eImg.Image = &lexutil.LexBlob{
			MimeType: "fake",
			Size:     0,
			Ref:      lexutil.LexLink(cid.Undef),
		}
		if uploadImages {
			output, err := atproto.RepoUploadBlob(ctx, xrpcClient, strings.NewReader(contents))
			if err != nil {
				return &Skeet{Err: err}
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

	skeet := &Skeet{
		x:   xrpcClient,
		p:   &post,
		Err: nil,
	}

	if len(images) > 0 && !uploadImages {
		skeet.Err = errors.New("images were not uploaded")
	}

	return skeet
}

func (s *Skeet) Publish(ctx context.Context) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}

	result, err := atproto.RepoCreateRecord(ctx, s.x, &atproto.RepoCreateRecord_Input{
		Collection: "app.bsky.feed.post",
		Repo:       s.x.Auth.Did,
		Record: &lexutil.LexiconTypeDecoder{
			Val: s.p,
		},
	})

	if err != nil {
		return "", err
	}

	return result.Uri, nil
}
