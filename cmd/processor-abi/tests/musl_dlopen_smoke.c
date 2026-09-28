#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>

typedef int (*trivial_answer_fn)(void);

int main(int argc, char **argv) {
	if (argc != 2) {
		fprintf(stderr, "usage: %s <shared-library>\n", argv[0]);
		return 2;
	}

	void *library = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
	if (library == NULL) {
		fprintf(stderr, "dlopen failed: %s\n", dlerror());
		return 1;
	}

	dlerror();
	trivial_answer_fn trivial_answer = (trivial_answer_fn)dlsym(library, "trivial_answer");
	const char *symbol_error = dlerror();
	if (symbol_error != NULL) {
		fprintf(stderr, "dlsym failed: %s\n", symbol_error);
		dlclose(library);
		return 1;
	}

	int answer = trivial_answer();
	dlclose(library);
	if (answer != 42) {
		fprintf(stderr, "trivial_answer returned %d, want 42\n", answer);
		return 1;
	}

	return 0;
}
