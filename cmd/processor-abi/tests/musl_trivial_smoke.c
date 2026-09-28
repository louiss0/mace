#include <assert.h>
#include <stdint.h>

extern uint64_t trivial_answer(void);

int main(void) {
    assert(trivial_answer() == 42);
    return 0;
}
