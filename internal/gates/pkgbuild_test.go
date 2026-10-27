package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageBuildHardening(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "PKGBUILD"))
	if err != nil {
		t.Fatal(err)
	}
	if p := checkPKGBUILD(b); len(p) > 0 {
		t.Fatalf("PKGBUILD:\n%s", strings.Join(p, "\n"))
	}
}

func TestPackageBuildGateHasTeeth(t *testing.T) {
	good := `depends=('polkit' 'glibc')

build() {
  cd "$pkgname-$pkgver"
  export CGO_ENABLED=0 GOFLAGS='-buildmode=pie -trimpath -mod=readonly -modcacherw'
  go build -ldflags '-bindnow' -o onceroot ./cmd/onceroot
  go build -ldflags '-bindnow' -o oncerootd ./cmd/oncerootd
}

check() {
  cd "$pkgname-$pkgver"
  go test ./...
  ONCEROOT_CHECK_BIN="$PWD/onceroot:$PWD/oncerootd" go test -count=1 -run '^TestBuiltBinariesAreHardened$' ./internal/gates/
}
`
	if p := checkPKGBUILD([]byte(good)); len(p) > 0 {
		t.Fatalf("a good PKGBUILD failed: %v", p)
	}
	for name, bad := range map[string]string{
		"no PIE":                   strings.Replace(good, "-buildmode=pie ", "", 1),
		"no BIND_NOW":              strings.Replace(good, "'-bindnow'", "''", 1),
		"stripped by the link":     strings.Replace(good, "'-bindnow'", "'-s -w -bindnow'", 1),
		"stripped, one flag":       strings.Replace(good, "'-bindnow'", "'-bindnow -s'", 1),
		"cgo on":                   strings.Replace(good, "CGO_ENABLED=0", "CGO_ENABLED=1", 1),
		"no trimpath":              strings.Replace(good, "-trimpath ", "", 1),
		"tests not run":            strings.Replace(good, "  go test ./...\n", "", 1),
		"no check()":               good[:strings.Index(good, "check()")],
		"tests run in build":       strings.Replace(strings.Replace(good, "  go test ./...\n", "", 1), "  go build", "  go test ./...\n  go build", 1),
		"polkit not a dep":         strings.Replace(good, "'polkit' ", "", 1),
		"one build loses BIND_NOW": strings.Replace(good, "-ldflags '-bindnow' -o oncerootd", `-ldflags "-X main.version=$pkgver" -o oncerootd`, 1),
		"one build not PIE":        strings.Replace(good, "go build -ldflags '-bindnow' -o oncerootd", "go build -buildmode=exe -ldflags '-bindnow' -o oncerootd", 1),
		"stripped, -s=true":        strings.Replace(good, "'-bindnow' -o oncerootd", "'-bindnow -s=true' -o oncerootd", 1),
		"stripped, --s":            strings.Replace(good, "'-bindnow' -o oncerootd", "'--s -bindnow' -o oncerootd", 1),
		"cgo turned on later":      strings.Replace(good, "  go build -ldflags '-bindnow' -o onceroot ", "  export CGO_ENABLED=1\n  go build -ldflags '-bindnow' -o onceroot ", 1),
		"a later weak build()":     good + "\nbuild() {\n  go build -o oncerootd ./cmd/oncerootd\n}\n",
		"binaries never measured":  strings.Replace(good, "  ONCEROOT_CHECK_BIN=", "  true ONCEROOT_CHECK=", 1),
		"empty file":               "",
	} {
		if p := checkPKGBUILD([]byte(bad)); len(p) == 0 {
			t.Errorf("%s passed", name)
		}
	}
}
