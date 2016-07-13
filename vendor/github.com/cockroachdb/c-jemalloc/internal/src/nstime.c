#include "jemalloc/internal/jemalloc_internal.h"

#define	BILLION	UINT64_C(1000000000)

void
nstime_init(nstime_t *time, uint64_t ns)
{

	time->ns = ns;
}

void
nstime_init2(nstime_t *time, uint64_t sec, uint64_t nsec)
{

	time->ns = sec * BILLION + nsec;
}

uint64_t
nstime_ns(const nstime_t *time)
{

	return (time->ns);
}

uint64_t
nstime_sec(const nstime_t *time)
{

	return (time->ns / BILLION);
}

uint64_t
nstime_nsec(const nstime_t *time)
{

	return (time->ns % BILLION);
}

void
nstime_copy(nstime_t *time, const nstime_t *source)
{

	*time = *source;
}

int
nstime_compare(const nstime_t *a, const nstime_t *b)
{

	return ((a->ns > b->ns) - (a->ns < b->ns));
}

void
nstime_add(nstime_t *time, const nstime_t *addend)
{

	assert(UINT64_MAX - time->ns >= addend->ns);

	time->ns += addend->ns;
}

void
nstime_subtract(nstime_t *time, const nstime_t *subtrahend)
{

	assert(nstime_compare(time, subtrahend) >= 0);

	time->ns -= subtrahend->ns;
}

void
nstime_imultiply(nstime_t *time, uint64_t multiplier)
{

	assert((((time->ns | multiplier) & (UINT64_MAX << (sizeof(uint64_t) <<
	    2))) == 0) || ((time->ns * multiplier) / multiplier == time->ns));

	time->ns *= multiplier;
}

void
nstime_idivide(nstime_t *time, uint64_t divisor)
{

	assert(divisor != 0);

	time->ns /= divisor;
}

uint64_t
nstime_divide(const nstime_t *time, const nstime_t *divisor)
{

	assert(divisor->ns != 0);

	return (time->ns / divisor->ns);
}

#ifdef JEMALLOC_JET
#undef nstime_update
#define	nstime_update JEMALLOC_N(nstime_update_impl)
#endif
bool
nstime_update(nstime_t *time)
{
	nstime_t old_time;

	nstime_copy(&old_time, time);

#ifdef _WIN32
	{
		FILETIME ft;
		uint64_t ticks;
		GetSystemTimeAsFileTime(&ft);
		ticks = (((uint64_t)ft.dwHighDateTime) << 32) |
		    ft.dwLowDateTime;
		time->ns = ticks * 100;
	}
#elif JEMALLOC_CLOCK_GETTIME
	{
		struct timespec ts;

		if (sysconf(_SC_MONOTONIC_CLOCK) > 0)
			clock_gettime(CLOCK_MONOTONIC, &ts);
		else
			clock_gettime(CLOCK_REALTIME, &ts);
		time->ns = ts.tv_sec * BILLION + ts.tv_nsec;
	}
#else
	struct timeval tv;
	gettimeofday(&tv, NULL);
	time->ns = tv.tv_sec * BILLION + tv.tv_usec * 1000;
#endif

	/* Handle non-monotonic clocks. */
	if (unlikely(nstime_compare(&old_time, time) > 0)) {
		nstime_copy(time, &old_time);
		return (true);
	}

	return (false);
}
#ifdef JEMALLOC_JET
#undef nstime_update
#define	nstime_update JEMALLOC_N(nstime_update)
nstime_update_t *nstime_update = JEMALLOC_N(nstime_update_impl);
#endif
