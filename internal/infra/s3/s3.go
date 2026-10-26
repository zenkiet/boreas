// Package s3 keeps objects in any S3-compatible bucket: Cloudflare R2, MinIO or AWS.
package s3

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/zenkiet/boreas/internal/core"
)

const objectLimit = 16 << 20

type Store struct {
	bucket string
	region string
	creds  aws.Credentials
	signer *v4.Signer
	http   *http.Client
}

// New addresses the bucket by path, which R2, MinIO and AWS all accept.
func New(endpoint, bucket, region, accessKeyID, secretAccessKey string) *Store {
	return &Store{
		bucket: strings.TrimSuffix(endpoint, "/") + "/" + bucket, region: cmp.Or(region, "auto"),
		creds:  aws.Credentials{AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey},
		signer: v4.NewSigner(), http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Store) Put(ctx context.Context, key, contentType string, body []byte) error {
	_, _, err := s.do(ctx, http.MethodPut, key, contentType, body)
	return err
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, string, error) {
	return s.do(ctx, http.MethodGet, key, "", nil)
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, _, err := s.do(ctx, http.MethodDelete, key, "", nil)
	return err
}

func (s *Store) do(ctx context.Context, method, key, contentType string, body []byte) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.bucket+"/"+key, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("build %s %s: %w", method, key, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	// S3 rejects a request whose signed headers lack the payload hash; SignHTTP does not add it.
	req.Header.Set("X-Amz-Content-Sha256", hash)
	if err := s.signer.SignHTTP(ctx, s.creds, req, hash, "s3", s.region, time.Now()); err != nil {
		return nil, "", fmt.Errorf("sign %s %s: %w", method, key, err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s object %s: %w", method, key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", core.ErrNotFound
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("%s object %s: %s", method, key, resp.Status)
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, objectLimit))
	if err != nil {
		return nil, "", fmt.Errorf("read object %s: %w", key, err)
	}
	return out, resp.Header.Get("Content-Type"), nil
}
