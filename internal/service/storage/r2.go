package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type R2Storage struct {
	client    *s3.Client
	bucket    string
	publicURL string
	ready     bool
}

func NewR2Storage(endpoint, accessKey, secretKey, region, bucket, publicURL string) *R2Storage {
	st := &R2Storage{bucket: bucket, publicURL: publicURL}

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		log.Println("WARN: S3/R2 storage not configured, uploads will use local storage")
		return st
	}

	resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, r string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: endpoint}, nil
		},
	)

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithEndpointResolverWithOptions(resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		log.Printf("WARN: Failed to configure S3/R2: %v", err)
		return st
	}

	st.client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	st.ready = true

	log.Printf("R2 Storage initialized (bucket: %s)", bucket)
	return st
}

func (s *R2Storage) IsReady() bool {
	return s.ready
}

func (s *R2Storage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if !s.ready {
		return "", fmt.Errorf("storage not configured")
	}

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("r2 upload error: %w", err)
	}

	url := fmt.Sprintf("%s/%s", strings.TrimRight(s.publicURL, "/"), key)
	return url, nil
}

func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("10.0.0.0/8")},
		{mustParseCIDR("172.16.0.0/12")},
		{mustParseCIDR("192.168.0.0/16")},
		{mustParseCIDR("127.0.0.0/8")},
		{mustParseCIDR("169.254.0.0/16")},
		{mustParseCIDR("::1/128")},
		{mustParseCIDR("fc00::/7")},
	}
	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, _ := net.ParseCIDR(s)
	return ipNet
}

func isValidSourceURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if isPrivateIP(host) {
		return false
	}
	if host == "localhost" {
		return false
	}
	return true
}

var ssrfSafeClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("ssrf: invalid address %q", addr)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("ssrf: dns lookup failed: %w", err)
			}
			for _, ip := range ips {
				if isPrivateIP(ip.IP.String()) {
					return nil, fmt.Errorf("ssrf: resolved to private IP")
				}
			}
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	},
}

func (s *R2Storage) UploadFromURL(ctx context.Context, sourceURL, folder string) (string, error) {
	if !s.ready {
		return sourceURL, nil
	}

	if sourceURL == "" {
		return "", fmt.Errorf("empty source URL")
	}

	if !isValidSourceURL(sourceURL) {
		return sourceURL, fmt.Errorf("r2: invalid or blocked source URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return sourceURL, fmt.Errorf("r2: create download request: %w", err)
	}

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return sourceURL, fmt.Errorf("r2: download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return sourceURL, fmt.Errorf("r2: download returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return sourceURL, fmt.Errorf("r2: read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	ext := guessExtension(contentType, sourceURL)

	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	key := fmt.Sprintf("%s/%s_%x%s", folder, time.Now().Format("20060102_150405"), randBytes, ext)

	url, err := s.Upload(ctx, key, data, contentType)
	if err != nil {
		return sourceURL, err
	}

	return url, nil
}

func guessExtension(contentType, url string) string {
	switch {
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		return ".jpg"
	case strings.Contains(contentType, "mp4"), strings.Contains(contentType, "video"):
		return ".mp4"
	case strings.Contains(contentType, "webm"):
		return ".webm"
	}

	ext := path.Ext(strings.Split(url, "?")[0])
	if ext != "" && len(ext) <= 5 {
		return ext
	}

	return ".jpg"
}