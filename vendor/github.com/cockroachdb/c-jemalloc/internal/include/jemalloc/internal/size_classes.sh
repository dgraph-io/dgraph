#!/bin/sh
#
# Usage: size_classes.sh <lg_qarr> <lg_tmin> <lg_parr> <lg_g>

# The following limits are chosen such that they cover all supported platforms.

# Pointer sizes.
lg_zarr="2 3"

# Quanta.
lg_qarr=$1

# The range of tiny size classes is [2^lg_tmin..2^(lg_q-1)].
lg_tmin=$2

# Maximum lookup size.
lg_kmax=12

# Page sizes.
lg_parr=`echo $3 | tr ',' ' '`

# Size class group size (number of size classes for each size doubling).
lg_g=$4

pow2() {
  e=$1
  pow2_result=1
  while [ ${e} -gt 0 ] ; do
    pow2_result=$((${pow2_result} + ${pow2_result}))
    e=$((${e} - 1))
  done
}

lg() {
  x=$1
  lg_result=0
  while [ ${x} -gt 1 ] ; do
    lg_result=$((${lg_result} + 1))
    x=$((${x} / 2))
  done
}

size_class() {
  index=$1
  lg_grp=$2
  lg_delta=$3
  ndelta=$4
  lg_p=$5
  lg_kmax=$6

  if [ ${lg_delta} -ge ${lg_p} ] ; then
    psz="yes"
  else
    pow2 ${lg_p}; p=${pow2_result}
    pow2 ${lg_grp}; grp=${pow2_result}
    pow2 ${lg_delta}; delta=${pow2_result}
    sz=$((${grp} + ${delta} * ${ndelta}))
    npgs=$((${sz} / ${p}))
    if [ ${sz} -eq $((${npgs} * ${p})) ] ; then
      psz="yes"
    else
      psz="no"
    fi
  fi

  lg ${ndelta}; lg_ndelta=${lg_result}; pow2 ${lg_ndelta}
  if [ ${pow2_result} -lt ${ndelta} ] ; then
    rem="yes"
  else
    rem="no"
  fi

  lg_size=${lg_grp}
  if [ $((${lg_delta} + ${lg_ndelta})) -eq ${lg_grp} ] ; then
    lg_size=$((${lg_grp} + 1))
  else
    lg_size=${lg_grp}
    rem="yes"
  fi

  if [ ${lg_size} -lt $((${lg_p} + ${lg_g})) ] ; then
    bin="yes"
  else
    bin="no"
  fi
  if [ ${lg_size} -lt ${lg_kmax} \
      -o ${lg_size} -eq ${lg_kmax} -a ${rem} = "no" ] ; then
    lg_delta_lookup=${lg_delta}
  else
    lg_delta_lookup="no"
  fi
  printf '    SC(%3d, %6d, %8d, %6d, %3s, %3s, %2s) \\\n' ${index} ${lg_grp} ${lg_delta} ${ndelta} ${psz} ${bin} ${lg_delta_lookup}
  # Defined upon return:
  # - psz ("yes" or "no")
  # - bin ("yes" or "no")
  # - lg_delta_lookup (${lg_delta} or "no")
}

sep_line() {
  echo "                                                         \\"
}

size_classes() {
  lg_z=$1
  lg_q=$2
  lg_t=$3
  lg_p=$4
  lg_g=$5

  pow2 $((${lg_z} + 3)); ptr_bits=${pow2_result}
  pow2 ${lg_g}; g=${pow2_result}

  echo "#define	SIZE_CLASSES \\"
  echo "  /* index, lg_grp, lg_delta, ndelta, psz, bin, lg_delta_lookup */ \\"

  ntbins=0
  nlbins=0
  lg_tiny_maxclass='"NA"'
  nbins=0
  npsizes=0

  # Tiny size classes.
  ndelta=0
  index=0
  lg_grp=${lg_t}
  lg_delta=${lg_grp}
  while [ ${lg_grp} -lt ${lg_q} ] ; do
    size_class ${index} ${lg_grp} ${lg_delta} ${ndelta} ${lg_p} ${lg_kmax}
    if [ ${lg_delta_lookup} != "no" ] ; then
      nlbins=$((${index} + 1))
    fi
    if [ ${psz} = "yes" ] ; then
      npsizes=$((${npsizes} + 1))
    fi
    if [ ${bin} != "no" ] ; then
      nbins=$((${index} + 1))
    fi
    ntbins=$((${ntbins} + 1))
    lg_tiny_maxclass=${lg_grp} # Final written value is correct.
    index=$((${index} + 1))
    lg_delta=${lg_grp}
    lg_grp=$((${lg_grp} + 1))
  done

  # First non-tiny group.
  if [ ${ntbins} -gt 0 ] ; then
    sep_line
    # The first size class has an unusual encoding, because the size has to be
    # split between grp and delta*ndelta.
    lg_grp=$((${lg_grp} - 1))
    ndelta=1
    size_class ${index} ${lg_grp} ${lg_delta} ${ndelta} ${lg_p} ${lg_kmax}
    index=$((${index} + 1))
    lg_grp=$((${lg_grp} + 1))
    lg_delta=$((${lg_delta} + 1))
    if [ ${psz} = "yes" ] ; then
      npsizes=$((${npsizes} + 1))
    fi
  fi
  while [ ${ndelta} -lt ${g} ] ; do
    size_class ${index} ${lg_grp} ${lg_delta} ${ndelta} ${lg_p} ${lg_kmax}
    index=$((${index} + 1))
    ndelta=$((${ndelta} + 1))
    if [ ${psz} = "yes" ] ; then
      npsizes=$((${npsizes} + 1))
    fi
  done

  # All remaining groups.
  lg_grp=$((${lg_grp} + ${lg_g}))
  while [ ${lg_grp} -lt $((${ptr_bits} - 1)) ] ; do
    sep_line
    ndelta=1
    if [ ${lg_grp} -eq $((${ptr_bits} - 2)) ] ; then
      ndelta_limit=$((${g} - 1))
    else
      ndelta_limit=${g}
    fi
    while [ ${ndelta} -le ${ndelta_limit} ] ; do
      size_class ${index} ${lg_grp} ${lg_delta} ${ndelta} ${lg_p} ${lg_kmax}
      if [ ${lg_delta_lookup} != "no" ] ; then
        nlbins=$((${index} + 1))
        # Final written value is correct:
        lookup_maxclass="((((size_t)1) << ${lg_grp}) + (((size_t)${ndelta}) << ${lg_delta}))"
      fi
      if [ ${psz} = "yes" ] ; then
        npsizes=$((${npsizes} + 1))
      fi
      if [ ${bin} != "no" ] ; then
        nbins=$((${index} + 1))
        # Final written value is correct:
        small_maxclass="((((size_t)1) << ${lg_grp}) + (((size_t)${ndelta}) << ${lg_delta}))"
        if [ ${lg_g} -gt 0 ] ; then
          lg_large_minclass=$((${lg_grp} + 1))
        else
          lg_large_minclass=$((${lg_grp} + 2))
        fi
      fi
      # Final written value is correct:
      huge_maxclass="((((size_t)1) << ${lg_grp}) + (((size_t)${ndelta}) << ${lg_delta}))"
      index=$((${index} + 1))
      ndelta=$((${ndelta} + 1))
    done
    lg_grp=$((${lg_grp} + 1))
    lg_delta=$((${lg_delta} + 1))
  done
  echo
  nsizes=${index}

  # Defined upon completion:
  # - ntbins
  # - nlbins
  # - nbins
  # - nsizes
  # - npsizes
  # - lg_tiny_maxclass
  # - lookup_maxclass
  # - small_maxclass
  # - lg_large_minclass
  # - huge_maxclass
}

cat <<EOF
/* This file was automatically generated by size_classes.sh. */
/******************************************************************************/
#ifdef JEMALLOC_H_TYPES

/*
 * This header requires LG_SIZEOF_PTR, LG_TINY_MIN, LG_QUANTUM, and LG_PAGE to
 * be defined prior to inclusion, and it in turn defines:
 *
 *   LG_SIZE_CLASS_GROUP: Lg of size class count for each size doubling.
 *   SIZE_CLASSES: Complete table of SC(index, lg_grp, lg_delta, ndelta, psz,
 *                 bin, lg_delta_lookup) tuples.
 *     index: Size class index.
 *     lg_grp: Lg group base size (no deltas added).
 *     lg_delta: Lg delta to previous size class.
 *     ndelta: Delta multiplier.  size == 1<<lg_grp + ndelta<<lg_delta
 *     psz: 'yes' if a multiple of the page size, 'no' otherwise.
 *     bin: 'yes' if a small bin size class, 'no' otherwise.
 *     lg_delta_lookup: Same as lg_delta if a lookup table size class, 'no'
 *                      otherwise.
 *   NTBINS: Number of tiny bins.
 *   NLBINS: Number of bins supported by the lookup table.
 *   NBINS: Number of small size class bins.
 *   NSIZES: Number of size classes.
 *   NPSIZES: Number of size classes that are a multiple of (1U << LG_PAGE).
 *   LG_TINY_MAXCLASS: Lg of maximum tiny size class.
 *   LOOKUP_MAXCLASS: Maximum size class included in lookup table.
 *   SMALL_MAXCLASS: Maximum small size class.
 *   LG_LARGE_MINCLASS: Lg of minimum large size class.
 *   HUGE_MAXCLASS: Maximum (huge) size class.
 */

#define	LG_SIZE_CLASS_GROUP	${lg_g}

EOF

for lg_z in ${lg_zarr} ; do
  for lg_q in ${lg_qarr} ; do
    lg_t=${lg_tmin}
    while [ ${lg_t} -le ${lg_q} ] ; do
      # Iterate through page sizes and compute how many bins there are.
      for lg_p in ${lg_parr} ; do
        echo "#if (LG_SIZEOF_PTR == ${lg_z} && LG_TINY_MIN == ${lg_t} && LG_QUANTUM == ${lg_q} && LG_PAGE == ${lg_p})"
        size_classes ${lg_z} ${lg_q} ${lg_t} ${lg_p} ${lg_g}
        echo "#define	SIZE_CLASSES_DEFINED"
        echo "#define	NTBINS			${ntbins}"
        echo "#define	NLBINS			${nlbins}"
        echo "#define	NBINS			${nbins}"
        echo "#define	NSIZES			${nsizes}"
        echo "#define	NPSIZES			${npsizes}"
        echo "#define	LG_TINY_MAXCLASS	${lg_tiny_maxclass}"
        echo "#define	LOOKUP_MAXCLASS		${lookup_maxclass}"
        echo "#define	SMALL_MAXCLASS		${small_maxclass}"
        echo "#define	LG_LARGE_MINCLASS	${lg_large_minclass}"
        echo "#define	HUGE_MAXCLASS		${huge_maxclass}"
        echo "#endif"
        echo
      done
      lg_t=$((${lg_t} + 1))
    done
  done
done

cat <<EOF
#ifndef SIZE_CLASSES_DEFINED
#  error "No size class definitions match configuration"
#endif
#undef SIZE_CLASSES_DEFINED
/*
 * The size2index_tab lookup table uses uint8_t to encode each bin index, so we
 * cannot support more than 256 small size classes.  Further constrain NBINS to
 * 255 since all small size classes, plus a "not small" size class must be
 * stored in 8 bits of arena_chunk_map_bits_t's bits field.
 */
#if (NBINS > 255)
#  error "Too many small size classes"
#endif

#endif /* JEMALLOC_H_TYPES */
/******************************************************************************/
#ifdef JEMALLOC_H_STRUCTS


#endif /* JEMALLOC_H_STRUCTS */
/******************************************************************************/
#ifdef JEMALLOC_H_EXTERNS


#endif /* JEMALLOC_H_EXTERNS */
/******************************************************************************/
#ifdef JEMALLOC_H_INLINES


#endif /* JEMALLOC_H_INLINES */
/******************************************************************************/
EOF
