package manifest_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"deps.dev/util/resolve"
	"deps.dev/util/resolve/dep"
	"github.com/google/osv-scanner/internal/resolution/manifest"
	"github.com/google/osv-scanner/internal/testutility"
	"github.com/google/osv-scanner/pkg/lockfile"
)

func aliasType(aliasedName string) dep.Type {
	t := dep.NewType()
	t.AddAttr(dep.KnownAs, aliasedName)

	return t
}

func npmVK(name, version string, versionType resolve.VersionType) resolve.VersionKey {
	return resolve.VersionKey{
		PackageKey: resolve.PackageKey{
			System: resolve.NPM,
			Name:   name,
		},
		Version:     version,
		VersionType: versionType,
	}
}

func TestNpmRead(t *testing.T) {
	t.Parallel()

	df, err := lockfile.OpenLocalDepFile("./fixtures/package.json")
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer df.Close()

	npmIO := manifest.NpmManifestIO{}
	got, err := npmIO.Read(df)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !strings.HasSuffix(got.FilePath, "package.json") {
		t.Errorf("manifest file path %v does not have package.json", got.FilePath)
	}
	got.FilePath = ""

	want := manifest.Manifest{
		Root: resolve.Version{
			VersionKey: npmVK("npm-manifest", "1.0.0", resolve.Concrete),
		},
		// npm dependencies should resolve in alphabetical order, regardless of type
		Requirements: []resolve.RequirementVersion{
			// TODO: @babel/core peerDependency currently not resolved
			{
				Type:       aliasType("cliui"), // sorts on aliased name, not real package name
				VersionKey: npmVK("@isaacs/cliui", "^8.0.2", resolve.Requirement),
			},
			{
				// Type: dep.NewType(dep.Dev), devDependencies treated as prod to make resolution work
				VersionKey: npmVK("eslint", "^8.57.0", resolve.Requirement),
			},
			{
				Type:       dep.NewType(dep.Opt),
				VersionKey: npmVK("glob", "^10.3.10", resolve.Requirement),
			},
			{
				VersionKey: npmVK("jquery", "latest", resolve.Requirement),
			},
			{
				VersionKey: npmVK("lodash", "4.17.17", resolve.Requirement),
			},
			{
				VersionKey: npmVK("string-width", "^5.1.2", resolve.Requirement),
			},
			{
				Type:       aliasType("string-width-aliased"),
				VersionKey: npmVK("string-width", "^4.2.3", resolve.Requirement),
			},
		},
		Groups: map[resolve.PackageKey][]string{
			{System: resolve.NPM, Name: "eslint"}: {"dev"},
			{System: resolve.NPM, Name: "glob"}:   {"optional"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("npm manifest mismatch:\ngot %v\nwant %v\n", got, want)
	}
}

func TestNpmWorkspaceRead(t *testing.T) {
	t.Parallel()

	df, err := lockfile.OpenLocalDepFile("./fixtures/npm-workspaces/package.json")
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer df.Close()

	npmIO := manifest.NpmManifestIO{}
	got, err := npmIO.Read(df)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !strings.HasSuffix(got.FilePath, "package.json") {
		t.Errorf("manifest file path %v does not have package.json", got.FilePath)
	}
	got.FilePath = ""
	for i, local := range got.LocalManifests {
		if !strings.HasSuffix(local.FilePath, "package.json") {
			t.Errorf("local manifest file path %v does not have package.json", local.FilePath)
		}
		got.LocalManifests[i].FilePath = ""
	}

	want := manifest.Manifest{
		Root: resolve.Version{
			VersionKey: npmVK("npm-workspace-test", "1.0.0", resolve.Concrete),
		},
		Requirements: []resolve.RequirementVersion{
			// root dependencies always before workspace
			{
				Type:       aliasType("jquery-real"),
				VersionKey: npmVK("jquery", "^3.7.1", resolve.Requirement),
			},
			// workspaces in path order
			{
				VersionKey: npmVK("jquery:workspace", "^3.7.1", resolve.Requirement),
			},
			{
				VersionKey: npmVK("@workspace/ugh:workspace", "*", resolve.Requirement),
			},
			{
				VersionKey: npmVK("z-z-z:workspace", "*", resolve.Requirement),
			},
		},
		Groups: map[resolve.PackageKey][]string{
			{System: resolve.NPM, Name: "jquery"}: {"dev"},
			// excludes workspace dev dependency
		},
		LocalManifests: []manifest.Manifest{
			{
				Root: resolve.Version{
					VersionKey: npmVK("jquery:workspace", "3.7.1", resolve.Concrete),
				},
				Requirements: []resolve.RequirementVersion{
					{
						VersionKey: npmVK("semver", "^7.6.0", resolve.Requirement),
					},
				},
				Groups: map[resolve.PackageKey][]string{},
			},
			{
				Root: resolve.Version{
					VersionKey: npmVK("@workspace/ugh:workspace", "0.0.1", resolve.Concrete),
				},
				Requirements: []resolve.RequirementVersion{
					{
						VersionKey: npmVK("jquery:workspace", "*", resolve.Requirement),
					},
					{
						VersionKey: npmVK("semver", "^6.3.1", resolve.Requirement),
					},
				},
				Groups: map[resolve.PackageKey][]string{
					{System: resolve.NPM, Name: "jquery:workspace"}: {"dev"},
					{System: resolve.NPM, Name: "semver"}:           {"dev"},
				},
			},
			{
				Root: resolve.Version{
					VersionKey: npmVK("z-z-z:workspace", "1.0.0", resolve.Concrete),
				},
				Requirements: []resolve.RequirementVersion{
					{
						VersionKey: npmVK("@workspace/ugh:workspace", "*", resolve.Requirement),
					},
					{
						VersionKey: npmVK("semver", "^5.7.2", resolve.Requirement),
					},
				},
				Groups: map[resolve.PackageKey][]string{},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("npm manifest mismatch:\ngot  %v\nwant %v\n", got, want)
	}
}

func TestNpmWrite(t *testing.T) {
	t.Parallel()

	df, err := lockfile.OpenLocalDepFile("./fixtures/package.json")
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer df.Close()

	changes := manifest.ManifestPatch{
		Deps: []manifest.DependencyPatch{
			{
				Pkg: resolve.PackageKey{
					System: resolve.NPM,
					Name:   "lodash",
				},
				OrigRequire: "4.17.17",
				NewRequire:  "^4.17.21",
			},
			{
				Pkg: resolve.PackageKey{
					System: resolve.NPM,
					Name:   "eslint",
				},
				OrigRequire: "^8.57.0",
				NewRequire:  "*",
			},
			{
				Pkg: resolve.PackageKey{
					System: resolve.NPM,
					Name:   "glob",
				},
				OrigRequire: "^10.3.10",
				NewRequire:  "^1.0.0",
			},
			{
				Pkg: resolve.PackageKey{
					System: resolve.NPM,
					Name:   "jquery",
				},
				OrigRequire: "latest",
				NewRequire:  "~0.0.1",
			},
			{
				Pkg: resolve.PackageKey{
					System: resolve.NPM,
					Name:   "@isaacs/cliui",
				},
				Type:        aliasType("cliui"),
				OrigRequire: "^8.0.2",
				NewRequire:  "^9.0.0",
			},
			{
				Pkg: resolve.PackageKey{
					System: resolve.NPM,
					Name:   "string-width",
				},
				OrigRequire: "^5.1.2",
				NewRequire:  "^7.1.0",
			},
			{
				Pkg: resolve.PackageKey{
					System: resolve.NPM,
					Name:   "string-width",
				},
				Type:        aliasType("string-width-aliased"),
				OrigRequire: "^4.2.3",
				NewRequire:  "^6.1.0",
			},
		},
	}

	buf := new(bytes.Buffer)
	npmIO := manifest.NpmManifestIO{}
	if err := npmIO.Write(df, buf, changes); err != nil {
		t.Fatalf("unable to update npm package.json: %v", err)
	}
	testutility.NewSnapshot().WithCRLFReplacement().MatchText(t, buf.String())
}
