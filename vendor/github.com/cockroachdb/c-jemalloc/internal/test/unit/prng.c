#include "test/jemalloc_test.h"

TEST_BEGIN(test_prng_lg_range)
{
	uint64_t sa, sb, ra, rb;
	unsigned lg_range;

	sa = 42;
	ra = prng_lg_range(&sa, 64);
	sa = 42;
	rb = prng_lg_range(&sa, 64);
	assert_u64_eq(ra, rb,
	    "Repeated generation should produce repeated results");

	sb = 42;
	rb = prng_lg_range(&sb, 64);
	assert_u64_eq(ra, rb,
	    "Equivalent generation should produce equivalent results");

	sa = 42;
	ra = prng_lg_range(&sa, 64);
	rb = prng_lg_range(&sa, 64);
	assert_u64_ne(ra, rb,
	    "Full-width results must not immediately repeat");

	sa = 42;
	ra = prng_lg_range(&sa, 64);
	for (lg_range = 63; lg_range > 0; lg_range--) {
		sb = 42;
		rb = prng_lg_range(&sb, lg_range);
		assert_u64_eq((rb & (UINT64_C(0xffffffffffffffff) << lg_range)),
		    0, "High order bits should be 0, lg_range=%u", lg_range);
		assert_u64_eq(rb, (ra >> (64 - lg_range)),
		    "Expected high order bits of full-width result, "
		    "lg_range=%u", lg_range);
	}
}
TEST_END

TEST_BEGIN(test_prng_range)
{
	uint64_t range;
#define	MAX_RANGE	10000000
#define	RANGE_STEP	97
#define	NREPS		10

	for (range = 2; range < MAX_RANGE; range += RANGE_STEP) {
		uint64_t s;
		unsigned rep;

		s = range;
		for (rep = 0; rep < NREPS; rep++) {
			uint64_t r = prng_range(&s, range);

			assert_u64_lt(r, range, "Out of range");
		}
	}
}
TEST_END

int
main(void)
{

	return (test(
	    test_prng_lg_range,
	    test_prng_range));
}
