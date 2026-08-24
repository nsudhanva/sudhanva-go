# sudhanva for Go

Minimal Go client for the public [sudhanva.me API](https://sudhanva.me/openapi.json). It retrieves
published profile and article metadata, performs bounded batch reads, searches the published site,
and creates or polls temporary profile-insight jobs.

The API is public and requires no credentials. Do not send private data.

## Install

```bash
go get github.com/nsudhanva/sudhanva-go@v0.1.0
```

## Use

```go
package main

import (
	"context"
	"fmt"

	sudhanva "github.com/nsudhanva/sudhanva-go"
)

func main() {
	client, err := sudhanva.NewClient()
	if err != nil {
		panic(err)
	}

	profile, err := client.Profile(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(profile.Profile.Name)
}
```

Profile-insight requests require a caller-controlled idempotency key:

```go
job, err := client.CreateProfileInsight(
	context.Background(),
	sudhanva.ProfileInsightRequest{
		Audience: "hiring-manager",
		Focus:    []string{"production-ml", "inference"},
	},
	"my-workflow-2026-08-23",
)
```

## API coverage

- `Profile`
- `Posts` and `Post`
- `Batch`
- `CreateProfileInsight`, `ProfileInsight`, and `WaitProfileInsight`
- `Ask` for NLWeb conversational search

The client follows the stable `/api/v1` contract. See the
[developer documentation](https://sudhanva.me/developers/sdks/) and
[versioning policy](https://sudhanva.me/developers/versioning/).

## Development

```bash
gofmt -w .
go test ./...
go vet ./...
```

Tests use local `httptest` servers and never call production.

## License

MIT
