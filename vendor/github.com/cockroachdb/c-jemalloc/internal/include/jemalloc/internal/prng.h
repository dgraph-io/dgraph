/******************************************************************************/
#ifdef JEMALLOC_H_TYPES

/*
 * Simple linear congruential pseudo-random number generator:
 *
 *   prng(y) = (a*x + c) % m
 *
 * where the following constants ensure maximal period:
 *
 *   a == Odd number (relatively prime to 2^n), and (a-1) is a multiple of 4.
 *   c == Odd number (relatively prime to 2^n).
 *   m == 2^32
 *
 * See Knuth's TAOCP 3rd Ed., Vol. 2, pg. 17 for details on these constraints.
 *
 * This choice of m has the disadvantage that the quality of the bits is
 * proportional to bit position.  For example, the lowest bit has a cycle of 2,
 * the next has a cycle of 4, etc.  For this reason, we prefer to use the upper
 * bits.
 */
#define	PRNG_A	UINT64_C(6364136223846793005)
#define	PRNG_C	UINT64_C(1442695040888963407)

#endif /* JEMALLOC_H_TYPES */
/******************************************************************************/
#ifdef JEMALLOC_H_STRUCTS

#endif /* JEMALLOC_H_STRUCTS */
/******************************************************************************/
#ifdef JEMALLOC_H_EXTERNS

#endif /* JEMALLOC_H_EXTERNS */
/******************************************************************************/
#ifdef JEMALLOC_H_INLINES

#ifndef JEMALLOC_ENABLE_INLINE
uint64_t	prng_lg_range(uint64_t *state, unsigned lg_range);
uint64_t	prng_range(uint64_t *state, uint64_t range);
#endif

#if (defined(JEMALLOC_ENABLE_INLINE) || defined(JEMALLOC_PRNG_C_))
JEMALLOC_ALWAYS_INLINE uint64_t
prng_lg_range(uint64_t *state, unsigned lg_range)
{
	uint64_t ret;

	assert(lg_range > 0);
	assert(lg_range <= 64);

	ret = (*state * PRNG_A) + PRNG_C;
	*state = ret;
	ret >>= (64 - lg_range);

	return (ret);
}

JEMALLOC_ALWAYS_INLINE uint64_t
prng_range(uint64_t *state, uint64_t range)
{
	uint64_t ret;
	unsigned lg_range;

	assert(range > 1);

	/* Compute the ceiling of lg(range). */
	lg_range = ffs_u64(pow2_ceil_u64(range)) - 1;

	/* Generate a result in [0..range) via repeated trial. */
	do {
		ret = prng_lg_range(state, lg_range);
	} while (ret >= range);

	return (ret);
}
#endif

#endif /* JEMALLOC_H_INLINES */
/******************************************************************************/
