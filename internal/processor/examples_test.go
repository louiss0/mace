package processor

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

var importExamples = map[string]string{
	"base.mace": `|======================================================|
from './profile.mace' import Profile:UserProfile, Age;
alias Name: string;

schema User: {
  name: Name,
  age: Age,
  profile?: UserProfile,
};

schema Secret: {
  token: int,
};

|======================================================|

[output = 'schema']
{
  Name: Name,
  User: User,
}`,
	"consumer.mace": `|===============================|
from './base.mace' import User;

string name = "Ada";

User result = {
  name: name,
  age: 27,
};
|===============================|

[output = 'data']
{ result: result, }`,
	"kebab.mace": `[output = 'data']
{
  display-name: "Ada",
}`,
	"optional_profile.mace": `[output = 'data']
{
  profile?: { city: "Paris", },
}`,
	"profile.mace": `|============================================|
alias Age: int;
schema Profile: { age: Age, bio?: string, };
|============================================|
[output = 'schema']
{
  Age: Age,
  Profile: Profile,
}`,
	"unguarded_optional_city.mace": `|===|
schema Address: {
  city: string,
};

schema Profile: {
  address?: Address,
};

schema User: {
  profile?: Profile,
};

User user = {
  profile: {
    address: {
      city: "Paris",
    },
  },
};
|===|

[output = 'data']
{
  city: user?.profile?.address?.city,
}`,
	"values.mace": `|===============|
int count = 3;
int hidden = 9;
|===============|
[output = 'data']
{
  count: count,
}`,
}

var unicodeImportExamples = map[string]string{
	"café.mace": `|===|
string value = "ok";
|===|
[output = 'data'] { value: value, }`,
	"import-canonical-equivalence/alias-consumer.mace": `|===|
from './exports.mace' import café:café;
|===|
[output = 'data'] { value: café, }`,
	"import-canonical-equivalence/bind-consumer.mace": `|===|
from './bind-source.mace' bind café;
|===|
[output = 'data'] { value: café.café, }`,
	"import-canonical-equivalence/bind-source.mace": `[output = 'data'] { café: "bound", }`,
	"import-canonical-equivalence/consumer.mace": `|===|
from './exports.mace' import café: local;
|===|
[output = 'data'] { value: local, }`,
	"import-canonical-equivalence/duplicate-aliases.mace": `|===|
from './exports.mace' import café:café, café:café;
|===|
[output = 'data'] {}`,
	"import-canonical-equivalence/exports.mace": `|===|
string café = "ok";
|===|
[output = 'data'] { café: café, }`,
	"import-canonical-equivalence/local-import-collision.mace": `|===|
from './exports.mace' import café;
string café = "local";
|===|
[output = 'data'] {}`,
	"import-canonical-equivalence/nfc-consumer.mace": `|===|
from './nfd-exports.mace' import café;
|===|
[output = 'data'] { value: café, }`,
	"import-canonical-equivalence/nfd-exports.mace": `|===|
string café = "nfd";
|===|
[output = 'data'] { café: café, }`,
	"import-canonical-equivalence/schema-file-consumer.mace": `[output = 'data', schema_file = './schema-file-source.mace', schema = café] { value: "ok", }`,
	"import-canonical-equivalence/schema-file-source.mace": `|===|
schema café: { value: string, };
|===|
[output = 'schema'] { café: café, }`,
	"path-not-normalized.mace": `|===|
from './café.mace' import value;
|===|
[output = 'data'] { value: value, }`,
}

func newExampleWorkspace(files map[string]string) string {
	workspace, err := os.MkdirTemp("", "mace-examples-*")
	tAssert.NoError(err)
	DeferCleanup(func() {
		_ = os.RemoveAll(workspace)
	})

	for path, contents := range files {
		writeExampleFile(workspace, path, contents)
	}

	return workspace
}

func processWithImportExamples(document string) (Result, error) {
	return New().ProcessInDir(document, newExampleWorkspace(importExamples))
}

func newDiagnosticWorkspace() string {
	workspace := newExampleWorkspace(nil)
	for name, source := range diagnosticExamples {
		writeExampleFile(workspace, name+".mace", source)
	}
	return workspace
}

func examplePath(root string, path string) string {
	return filepath.Join(root, filepath.FromSlash(path))
}
