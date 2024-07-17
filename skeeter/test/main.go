// A generated module for Test functions
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
	"dagger/test/internal/dagger"
	"fmt"
	"time"
)

type Test struct{}

func (m *Test) PublishTestSkeet(ctx context.Context, username string, appPassword *dagger.Secret) error {
	uri, err := dag.Skeeter().
		WithUsername(username).
		WithAppPassword(appPassword).
		Publish(ctx, fmt.Sprintf("Test post from https://dagger.io module at %s", time.Now().Format(time.RFC3339)))

	if err != nil {
		return err
	}

	fmt.Println(uri)
	return nil
}
