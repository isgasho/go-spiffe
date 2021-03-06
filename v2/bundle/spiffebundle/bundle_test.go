package spiffebundle_test

import (
	"crypto"
	"crypto/x509"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/bundle/jwtbundle"
	"github.com/spiffe/go-spiffe/v2/bundle/spiffebundle"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	filePath, err  string
	refreshHint    time.Duration
	sequenceNumber uint64
	keysCount      int
}

var (
	td        = spiffeid.RequireTrustDomainFromString("example.org")
	x509Cert1 = &x509.Certificate{
		Raw: []byte("CERT 1"),
	}
	x509Cert2 = &x509.Certificate{
		Raw: []byte("CERT 2"),
	}
	testCases = []testCase{
		{
			filePath: "testdata/does-not-exist.json",
		},
		{
			filePath:  "testdata/spiffebundle_valid_1.json",
			keysCount: 1,
		},
		{
			filePath:       "testdata/spiffebundle_valid_2.json",
			keysCount:      6,
			refreshHint:    60 * time.Second,
			sequenceNumber: 1,
		},
		{
			filePath: "testdata/spiffebundle_missing_kid.json",
			err:      "spiffebundle: error adding authority 1 of JWKS: keyID cannot be empty",
		},
		{
			filePath: "testdata/spiffebundle_no_keys.json",
			err:      "spiffebundle: no authorities found",
		},
		{
			filePath: "testdata/spiffebundle_multiple_x509.json",
			err:      "spiffebundle: expected a single certificate in x509-svid entry 0; got 2",
		},
	}
)

func TestNew(t *testing.T) {
	b := spiffebundle.New(td)
	require.NotNil(t, b)
	require.Len(t, b.JWTAuthorities(), 0)
	require.Equal(t, td, b.TrustDomain())
}

func TestLoad(t *testing.T) {
	testCases[0].err = "spiffebundle: unable to read SPIFFE bundle: open testdata/does-not-exist.json: no such file or directory"

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.filePath, func(t *testing.T) {
			bundle, err := spiffebundle.Load(td, testCase.filePath)
			checkBundleProperties(t, err, testCase, bundle)
		})
	}
}

func TestRead(t *testing.T) {
	testCases[0].err = "spiffebundle: unable to read: invalid argument"

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.filePath, func(t *testing.T) {
			// we expect the Open call to fail in some cases
			file, _ := os.Open(testCase.filePath)
			defer file.Close()

			bundle, err := spiffebundle.Read(td, file)
			checkBundleProperties(t, err, testCase, bundle)
		})
	}
}

func TestParse(t *testing.T) {
	testCases[0].err = "spiffebundle: unable to parse JWKS: unexpected end of JSON input"

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.filePath, func(t *testing.T) {
			// we expect the ReadFile call to fail in some cases
			bundleBytes, _ := ioutil.ReadFile(testCase.filePath)

			bundle, err := spiffebundle.Parse(td, bundleBytes)
			checkBundleProperties(t, err, testCase, bundle)
		})
	}
}

func TestFromX509Bundle(t *testing.T) {
	xb := x509bundle.FromX509Authorities(td, []*x509.Certificate{x509Cert1})
	sb := spiffebundle.FromX509Bundle(xb)
	require.NotNil(t, sb)
	assert.Equal(t, xb.X509Authorities(), sb.X509Authorities())
}

func TestFromJWTBundle(t *testing.T) {
	jb := jwtbundle.New(td)
	err := jb.AddJWTAuthority("key-1", "test-1")
	require.NoError(t, err)
	sb := spiffebundle.FromJWTBundle(jb)
	require.NotNil(t, sb)
	assert.Equal(t, jb.JWTAuthorities(), sb.JWTAuthorities())
}

func TestFromX509Authorities(t *testing.T) {
	x509Authorities := []*x509.Certificate{x509Cert1, x509Cert2}
	b := spiffebundle.FromX509Authorities(td, x509Authorities)
	require.NotNil(t, b)
	assert.Equal(t, b.X509Authorities(), x509Authorities)
}

func TestFromJWTAuthorities(t *testing.T) {
	jwtAuthorities := map[string]crypto.PublicKey{
		"key-1": "test-1",
		"key-2": "test-2",
	}
	b := spiffebundle.FromJWTAuthorities(td, jwtAuthorities)
	require.NotNil(t, b)
	assert.Equal(t, b.JWTAuthorities(), jwtAuthorities)
}

func TestTrustDomain(t *testing.T) {
	b := spiffebundle.New(td)
	btd := b.TrustDomain()
	require.Equal(t, td, btd)
}

func TestJWTAuthoritiesCRUD(t *testing.T) {
	// Test AddJWTAuthority (missing authority)
	b := spiffebundle.New(td)
	err := b.AddJWTAuthority("", "test-1")
	require.EqualError(t, err, "spiffebundle: keyID cannot be empty")

	// Test AddJWTAuthority (new authority)
	err = b.AddJWTAuthority("key-1", "test-1")
	require.NoError(t, err)

	// Test JWTAuthorities
	jwtAuthorities := b.JWTAuthorities()
	require.Equal(t, map[string]crypto.PublicKey{"key-1": "test-1"}, jwtAuthorities)

	err = b.AddJWTAuthority("key-2", "test-2")
	require.NoError(t, err)

	jwtAuthorities = b.JWTAuthorities()
	require.Equal(t, map[string]crypto.PublicKey{
		"key-1": "test-1",
		"key-2": "test-2",
	}, jwtAuthorities)

	// Test FindJWTAuthority
	jwtAuthority, ok := b.FindJWTAuthority("key-1")
	require.True(t, ok)
	require.Equal(t, "test-1", jwtAuthority)

	jwtAuthority, ok = b.FindJWTAuthority("key-3")
	require.Nil(t, jwtAuthority)
	require.False(t, ok)

	require.Equal(t, true, b.HasJWTAuthority("key-1"))
	b.RemoveJWTAuthority("key-3")

	require.Equal(t, 2, len(b.JWTAuthorities()))
	require.Equal(t, true, b.HasJWTAuthority("key-1"))
	require.Equal(t, true, b.HasJWTAuthority("key-2"))

	// Test RemoveJWTAuthority
	b.RemoveJWTAuthority("key-2")
	require.Equal(t, 1, len(b.JWTAuthorities()))
	require.Equal(t, true, b.HasJWTAuthority("key-1"))

	// Test AddJWTAuthority (update authority)
	err = b.AddJWTAuthority("key-1", "test-1-updated")
	require.NoError(t, err)
	jwtAuthorities = b.JWTAuthorities()
	require.Equal(t, map[string]crypto.PublicKey{
		"key-1": "test-1-updated",
	}, jwtAuthorities)
}

func TestX509AuthoritiesCRUD(t *testing.T) {
	// Test X509Authorities and HasX509Authority
	b := spiffebundle.New(td)
	require.Len(t, b.X509Authorities(), 0)
	require.Equal(t, false, b.HasX509Authority(x509Cert1))

	// Test AddX509Authority
	b.AddX509Authority(x509Cert1)
	require.Len(t, b.X509Authorities(), 1)
	require.Equal(t, true, b.HasX509Authority(x509Cert1))

	b.AddX509Authority(x509Cert1)
	require.Len(t, b.X509Authorities(), 1)
	require.Equal(t, true, b.HasX509Authority(x509Cert1))

	b.AddX509Authority(x509Cert2)
	require.Len(t, b.X509Authorities(), 2)
	require.Equal(t, true, b.HasX509Authority(x509Cert2))

	// Test RemoveX509Authority
	b.RemoveX509Authority(x509Cert1)
	require.Len(t, b.X509Authorities(), 1)
	require.Equal(t, true, b.HasX509Authority(x509Cert2))

	b.RemoveX509Authority(x509Cert2)
	require.Len(t, b.X509Authorities(), 0)
}

func TestRefreshHint(t *testing.T) {
	b := spiffebundle.New(td)
	rh, ok := b.RefreshHint()
	assert.Equal(t, false, ok)
	assert.Equal(t, time.Duration(0), rh)

	b.SetRefreshHint(30 * time.Second)
	rh, ok = b.RefreshHint()
	assert.Equal(t, true, ok)
	assert.Equal(t, 30*time.Second, rh)

	b.ClearRefreshHint()
	rh, ok = b.RefreshHint()
	assert.Equal(t, false, ok)
	assert.Equal(t, time.Duration(0), rh)
}

func TestSequenceNumber(t *testing.T) {
	b := spiffebundle.New(td)
	sn, ok := b.SequenceNumber()
	assert.Equal(t, false, ok)
	assert.Equal(t, uint64(0), sn)

	b.SetSequenceNumber(5)
	sn, ok = b.SequenceNumber()
	assert.Equal(t, true, ok)
	assert.Equal(t, uint64(5), sn)

	b.ClearSequenceNumber()
	sn, ok = b.SequenceNumber()
	assert.Equal(t, false, ok)
	assert.Equal(t, uint64(0), sn)
}

func TestMarshal(t *testing.T) {
	// Load a bundle to marshal
	bundle, err := spiffebundle.Load(td, "testdata/spiffebundle_valid_2.json")
	require.NoError(t, err)

	// Marshal the bundle
	bundleBytesMarshal, err := bundle.Marshal()
	require.NoError(t, err)

	// Parse the marshaled bundle
	bundleParsed, err := spiffebundle.Parse(td, bundleBytesMarshal)
	require.NoError(t, err)

	// Assert that the marshaled bundle is equal to the parsed bundle
	assert.Equal(t, bundleParsed, bundle)
}

func TestX509Bundle(t *testing.T) {
	sb := spiffebundle.New(td)
	sb.AddX509Authority(x509Cert1)
	xb := sb.X509Bundle()
	require.Equal(t, true, xb.HasX509Authority(x509Cert1))
}

func TestJWTBundle(t *testing.T) {
	sb := spiffebundle.New(td)
	err := sb.AddJWTAuthority("key-1", "test-1")
	require.NoError(t, err)
	jb := sb.JWTBundle()
	require.Equal(t, true, jb.HasJWTAuthority("key-1"))
}

func TestGetBundleForTrustDomain(t *testing.T) {
	b := spiffebundle.New(td)
	b1, err := b.GetBundleForTrustDomain(td)
	require.NoError(t, err)
	require.Equal(t, b, b1)

	td2 := spiffeid.RequireTrustDomainFromString("example-2.org")
	b2, err := b.GetBundleForTrustDomain(td2)
	require.Nil(t, b2)
	require.EqualError(t, err, `spiffebundle: no SPIFFE bundle for trust domain "example-2.org"`)
}

func TestGetX509BundleForTrustDomain(t *testing.T) {
	xb1 := x509bundle.FromX509Authorities(td, []*x509.Certificate{x509Cert1, x509Cert2})
	sb := spiffebundle.FromX509Bundle(xb1)
	xb2, err := sb.GetX509BundleForTrustDomain(td)
	require.NoError(t, err)
	require.Equal(t, xb1, xb2)

	td2 := spiffeid.RequireTrustDomainFromString("example-2.org")
	xb2, err = sb.GetX509BundleForTrustDomain(td2)
	require.Nil(t, xb2)
	require.EqualError(t, err, `spiffebundle: no X.509 bundle for trust domain "example-2.org"`)
}

func TestGetJWTBundleForTrustDomain(t *testing.T) {
	jb1 := jwtbundle.FromJWTAuthorities(td, map[string]crypto.PublicKey{"key-1": "test-1"})
	sb := spiffebundle.FromJWTBundle(jb1)
	jb2, err := sb.GetJWTBundleForTrustDomain(td)
	require.NoError(t, err)
	require.Equal(t, jb1, jb2)

	td2 := spiffeid.RequireTrustDomainFromString("example-2.org")
	jb2, err = sb.GetJWTBundleForTrustDomain(td2)
	require.Nil(t, jb2)
	require.EqualError(t, err, `spiffebundle: no JWT bundle for trust domain "example-2.org"`)
}

func checkBundleProperties(t *testing.T, err error, tc testCase, b *spiffebundle.Bundle) {
	if tc.err != "" {
		require.EqualError(t, err, tc.err)
		return
	}
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Len(t, b.JWTAuthorities(), tc.keysCount)
	rh, ok := b.RefreshHint()
	if tc.refreshHint > 0 {
		assert.Equal(t, true, ok)
		assert.Equal(t, tc.refreshHint, rh)
	}
	sn, ok := b.SequenceNumber()
	if tc.sequenceNumber > 0 {
		assert.Equal(t, true, ok)
		assert.Equal(t, tc.sequenceNumber, sn)
	}
}
