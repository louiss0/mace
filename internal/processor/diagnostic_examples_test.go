package processor

var diagnosticExamples = map[string]string{
	"choice-alias-cycle": `|===|
alias First: fusion[Second];
alias Second: fusion[First];
|===|
[output = 'data'] {}
`,
	"choice-duplicate-member": `|===|
alias Status: choice['ready', 'ready'];
|===|
[output = 'data'] {}
`,
	"choice-invalid-member": `|===|
alias Other: string;
alias Invalid: choice[Other];
|===|
{}
`,
	"circular-import-a": `|===|
from './circular-import-b.mace' import Name;
alias Name: string;
|===|
[output = 'data'] {}`,
	"circular-import-b": `|===|
from './circular-import-a.mace' import Name;
alias Name: string;
|===|
[output = 'data'] {}
`,
	"directive-list-missing-output": `[schema = Missing]
{}
`,
	"division-by-zero": `|===|
int value = 1 / 0;
|===|
[output = 'data'] { value, }
`,
	"documentation-before-target": `|===|
gen_doc Name {
  summary: 'Declared too early',
};
alias Name: string;
|===|
[output = 'data'] {}
`,
	"documentation-conflict": `|===|
alias Name: string /# Inline documentation;
gen_doc Name {
  summary: 'Structured documentation',
};
|===|
[output = 'data'] {}
`,
	"documentation-fields-on-gen-doc": `|===|
alias Name: string;
gen_doc Name {
  fields: { value: 'Invalid', },
};
|===|
[output = 'data'] {}
`,
	"documentation-unknown-field": `|===|
schema User: { name: string, };
schema_doc User {
  fields: { age: 'Unknown field', },
};
|===|
[output = 'data'] {}
`,
	"duplicate-declaration": `|===|
alias User: string;
schema User: { name: string, };
|===|
[output = 'data'] {}`,
	"duplicate-directive": `[output = 'data', output = 'schema']
{}
`,
	"duplicate-import": `|===|
from './support/import-source.mace' import Exposed;
from './support/import-source.mace' import Exposed;
|===|
[output = 'data'] {}
`,
	"duplicate-output-field": `[output = 'data']
{ value: 1, value: 2, }
`,
	"duplicate-schema-field": `|===|
schema User: { name: string, name: string, };
|===|
[output = 'data'] {}
`,
	"empty-script-block": `|===|
|===|
[output = 'data'] {}
`,
	"fusion-field-conflict": `|===|
schema Left: { value: string, };
schema Right: { value: int, };
alias Invalid: fusion[Left, Right];
|===|
[output = 'data'] {}
`,
	"fusion-mixed-kinds": `|===|
alias Status: choice['ready', 'pending'];
schema User: { name: string, };
alias Invalid: fusion[Status, User];
|===|
[output = 'data'] {}
`,
	"fusion-scalar-member": `|===|
alias Invalid: fusion[string, { name: string, }];
|===|
[output = 'data'] {}
`,
	"gen-doc-invalid-target": `|===|
schema User: { name: string, };
User profile = { name: 'Ada', };

gen_doc profile {
  summary: "Invalid target.",
};
|===|
[output = 'data'] {}
`,
	"import-after-declaration": `|===|
string local = 'value';
from './support/import-source.mace' import Exposed;
|===|
[output = 'data'] { local, }
`,
	"import-missing-terminator": `|===|
from './support/import-source.mace' import Shared
|===|
{}
`,
	"import-name-not-exposed": `|===|
from './support/import-source.mace' import Hidden;
|===|
[output = 'data'] {}
`,
	"import-shadows-local": `|===|
from './support/import-source.mace' import Exposed;
string Exposed = 'local';
|===|
[output = 'data'] { Exposed, }
`,
	"inconsistent-script-delimiters": `|===|
string value = 'invalid';
|====|
[output = 'data'] { value, }
`,
	"integer-overflow": `|===|
int value = 9223372036854775808;
|===|
[output = 'data'] { value, }
`,
	"interpolation-rejects-non-primitives": `|===|
schema User: {
  name: string,
};

User user = {
  name: "Ada",
};

string message = "Hello $(user)";
|===|
[output = 'data']
{
  result: "unreachable",
}
`,
	"invalid-bitwise-complement": `|===|
int value = ~1.5;
|===|
[output = 'data'] { value, }
`,
	"invalid-path-extension": `|===|
from './support/import-source' import Exposed;
|===|
[output = 'data'] {}
`,
	"match-choice-type-pattern": `|===|
choice['on', 'off'] value = 'on';
int selected = match (value) {
  string => 1,
  'off' => 0,
};
|===|
[output = 'data'] { selected, }
`,
	"match-concrete-input": `|===|
string value = 'Mace';
string selected = match (value) {
  string => 'text',
};
|===|
[output = 'data'] { selected, }
`,
	"match-duplicate-pattern": `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text',
  string => 'again',
  int => 'number',
};
|===|
[output = 'data'] { selected, }
`,
	"match-non-exhaustive": `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text',
};
|===|
[output = 'data'] { selected, }
`,
	"match-unknown-pattern": `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text',
  boolean => 'flag',
  int => 'number',
};
|===|
[output = 'data'] { selected, }
`,
	"match-variant-literal-pattern": `|===|
variant[string, int] value = 1;
string selected = match (value) {
  'text' => 'text',
  int => 'number',
};
|===|
[output = 'data'] { selected, }
`,
	"missing-field-separator": `[output = 'data']
{
  first: 1
  second: 2,
}
`,
	"missing-output-block": `|===|
string value = 'no output';
|===|
`,
	"missing-runtime-input": `|===|
schema Input: { name: string, };
|===|
[output = 'data', parse = Input]
{ result: 'unreachable', }
`,
	"mixed-array-literal": `|===|
array<int> values = [1, 'two'];
|===|
[output = 'data'] { values, }
`,
	"mixed-decimal-hexadecimal-operands": `|===|
int value = 1 + 0x1;
|===|
[output = 'data'] { value, }
`,
	"multiple-output-blocks": `[output = 'data'] {}
[output = 'data'] {}
`,
	"nested-conditional": `|===|
boolean enabled = true;
string value = enabled ? enabled ? 'a' : 'b' : 'c';
|===|
[output = 'data'] { value, }
`,
	"null-array-member": `|===|
array<string> values = [null];
|===|
[output = 'data'] { values, }
`,
	"null-comparison": `|===|
string value = null;
boolean invalid = value == null;
|===|
[output = 'data'] { invalid, }
`,
	"null-interpolation": `|===|
string value = null;
string invalid = "$(value)";
|===|
[output = 'data'] { invalid, }
`,
	"null-output": `[output = 'data']
{ value: null, }
`,
	"null-record-member": `|===|
{ value: string, } item = { value: null, };
|===|
[output = 'data'] { item, }
`,
	"nullable-unguarded-access": `|===|
schema User: { name: string, };
User user = null;
string name = user.name;
|===|
[output = 'data'] { name, }
`,
	"optional-chain-unresolved": `|===|
schema User: { name?: string, };
User user = {};
string name = user?.name;
|===|
[output = 'data'] { name, }
`,
	"optional-data-field": `|===|
schema User: { name: string, };
|===|
[output = 'data', schema = User]
{ name?: 'Ada', }
`,
	"optional-field-plain-access": `|===|
schema User: { name?: string, };
User user = {};
string name = user.name;
|===|
[output = 'data'] { name, }
`,
	"output-inline-doc-requires-directive-list": `[output = 'data', description = 42]
{
}
`,
	"output-shorthand-rejects-missing-variable": `[output = 'data']
{
  missing,
}
`,
	"path-escape": `|===|
from '../arrays.mace' import Missing;
|===|
[output = 'data'] {}
`,
	"record-access-past-depth": `|===|
record<string> packages = {};
string value = packages.codefixer.cn_efs;
|===|
[output = 'data'] { value, }
`,
	"schema-directive-in-schema-output": `|===|
schema User: { name: string, };
|===|
[output = 'schema', schema = User]
{ name: string, }
`,
	"schema-doc-duplicate-keys": `|===|
schema User: { name: string, };

schema_doc User {
  summary: "One",
  summary: "Two",
};
|===|
[output = 'data'] {}`,
	"schema-doc-invalid-target": `|===|
alias Status: string;

schema_doc Status {
  summary: "Invalid target.",
};
|===|
[output = 'data'] {}`,
	"schema-doc-scalar-target": `|===|
string greeting = "Hello";

schema_doc greeting {
  summary: "Invalid target.",
};
|===|
[output = 'data'] {}`,
	"schema-missing-required-field": `|===|
schema User: { name: string, age: int, };
|===|
[output = 'data', schema = User]
{ name: 'Ada', }
`,
	"schema-missing-terminator": `|===|
schema Empty: {}
|===|
{}
`,
	"schema-unknown-field": `|===|
schema User: { name: string, };
|===|
[output = 'data', schema = User]
{ name: 'Ada', extra: true, }
`,
	"self-direct-recursion": `|===|
schema Invalid: { self: $self, };
|===|
[output = 'data'] {}
`,
	"self-forward-reference": `[output = 'data']
{
  first: $self.second,
  second: 2,
}
`,
	"self-outside-schema": `|===|
alias Invalid: array<$self>;
|===|
[output = 'data'] {}
`,
	"self-unknown-field": `[output = 'data']
{ value: $self.missing, }
`,
	"shorthand-rejects-missing-variable": `|===|
schema User: { name: string, };
User user = { missing, };
|===|
[output = 'data']
{
  user,
}
`,
	"shorthand-rejects-nullable-required-field": `|===|
string name = null;
schema User: { name: string, };
User user = { name, };
|===|
[output = 'data']
{
  user,
}
`,
	"support/import-source": `|===|
alias Exposed: string;
alias Hidden: string;
|===|
[output = 'schema']
{ Exposed: Exposed, }
`,
	"type-alias-cycle": `|===|
alias First: Second;
alias Second: First;
|===|
[output = 'data'] {}
`,
	"type-mismatch": `|===|
int total = 1.5;
|===|
[output = 'data'] {}`,
	"unknown-directive": `[output = 'data', unknown = 'value']
{}
`,
	"unknown-type": `|===|
Unknown value = 1;
|===|
[output = 'data'] {}`,
	"unterminated-script-block": `|===|
string value = 'unterminated';
`,
	"untyped-empty-array-output": `[output = 'data']
{ value: [], }
`,
	"untyped-empty-record-output": `[output = 'data']
{ value: {}, }
`,
	"variable-in-schema-output": `|===|
string name = 'Ada';
|===|
[output = 'schema']
{ name: name, }
`,
	"variable-missing-initializer": `|===|
string value;
|===|
[output = 'data'] {}
`,
	"variant-duplicate-member": `|===|
alias Invalid: variant[string, string];
|===|
[output = 'data'] {}
`,
	"wildcard-import": `|===|
from './support/import-source.mace' import *;
|===|
[output = 'data'] {}
`,
}
