#include "processor.h"
#include <assert.h>

int main(void) {
    assert(mace_abi_major() == 1);
    return 0;
}
