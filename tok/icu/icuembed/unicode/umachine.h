/*
******************************************************************************
*
*   Copyright (C) 1999-2015, International Business Machines
*   Corporation and others.  All Rights Reserved.
*
******************************************************************************
*   file name:  umachine.h
*   encoding:   US-ASCII
*   tab size:   8 (not used)
*   indentation:4
*
*   created on: 1999sep13
*   created by: Markus W. Scherer
*
*   This file defines basic types and constants for ICU to be
*   platform-independent. umachine.h and utf.h are included into
*   utypes.h to provide all the general definitions for ICU.
*   All of these definitions used to be in utypes.h before
*   the UTF-handling macros made this unmaintainable.
*/

#ifndef __UMACHINE_H__
#define __UMACHINE_H__


/**
 * \file
 * \brief Basic types and constants for UTF
 *
 * <h2> Basic types and constants for UTF </h2>
 *   This file defines basic types and constants for utf.h to be
 *   platform-independent. umachine.h and utf.h are included into
 *   utypes.h to provide all the general definitions for ICU.
 *   All of these definitions used to be in utypes.h before
 *   the UTF-handling macros made this unmaintainable.
 *
 */
/*==========================================================================*/
/* Include platform-dependent definitions                                   */
/* which are contained in the platform-specific file platform.h             */
/*==========================================================================*/

#include "unicode/ptypes.h" /* platform.h is included in ptypes.h */

/*
 * ANSI C headers:
 * stddef.h defines wchar_t
 */
#include <stddef.h>

/*==========================================================================*/
/* For C wrappers, we use the symbol U_STABLE.                                */
/* This works properly if the includer is C or C++.                         */
/* Functions are declared   U_STABLE return-type U_EXPORT2 function-name()... */
/*==========================================================================*/

/**
 * \def U_CFUNC
 * This is used in a declaration of a library private ICU C function.
 * @stable ICU 2.4
 */

/**
 * \def U_CDECL_BEGIN
 * This is used to begin a declaration of a library private ICU C API.
 * @stable ICU 2.4
 */

/**
 * \def U_CDECL_END
 * This is used to end a declaration of a library private ICU C API
 * @stable ICU 2.4
 */

#ifdef __cplusplus
#   define U_CFUNC extern "C"
#   define U_CDECL_BEGIN extern "C" {
#   define U_CDECL_END   }
#else
#   define U_CFUNC extern
#   define U_CDECL_BEGIN
#   define U_CDECL_END
#endif

#ifndef U_ATTRIBUTE_DEPRECATED
/**
 * \def U_ATTRIBUTE_DEPRECATED
 *  This is used for GCC specific attributes
 * @internal
 */
#if U_GCC_MAJOR_MINOR >= 302
#    define U_ATTRIBUTE_DEPRECATED __attribute__ ((deprecated))
/**
 * \def U_ATTRIBUTE_DEPRECATED
 * This is used for Visual C++ specific attributes 
 * @internal
 */
#elif defined(_MSC_VER) && (_MSC_VER >= 1400)
#    define U_ATTRIBUTE_DEPRECATED __declspec(deprecated)
#else
#    define U_ATTRIBUTE_DEPRECATED
#endif
#endif

/** This is used to declare a function as a public ICU C API @stable ICU 2.0*/
#define U_CAPI U_CFUNC U_EXPORT
/** This is used to declare a function as a stable public ICU C API*/
#define U_STABLE U_CAPI
/** This is used to declare a function as a draft public ICU C API  */
#define U_DRAFT  U_CAPI
/** This is used to declare a function as a deprecated public ICU C API  */
#define U_DEPRECATED U_CAPI U_ATTRIBUTE_DEPRECATED
/** This is used to declare a function as an obsolete public ICU C API  */
#define U_OBSOLETE U_CAPI
/** This is used to declare a function as an internal ICU C API  */
#define U_INTERNAL U_CAPI

/**
 * \def U_OVERRIDE
 * Defined to the C++11 "override" keyword if available.
 * Denotes a class or member which is an override of the base class.
 * May result in an error if it applied to something not an override.
 * @internal
 */

/**
 * \def U_FINAL
 * Defined to the C++11 "final" keyword if available.
 * Denotes a class or member which may not be overridden in subclasses.
 * May result in an error if subclasses attempt to override.
 * @internal
 */

#if U_CPLUSPLUS_VERSION >= 11
/* C++11 */
#ifndef U_OVERRIDE
#define U_OVERRIDE override
#endif
#ifndef U_FINAL
#define U_FINAL final
#endif
#else
/* not C++11 - define to nothing */
#ifndef U_OVERRIDE
#define U_OVERRIDE
#endif
#ifndef U_FINAL
#define U_FINAL
#endif
#endif

/*==========================================================================*/
/* limits for int32_t etc., like in POSIX inttypes.h                        */
/*==========================================================================*/

#ifndef INT8_MIN
/** The smallest value an 8 bit signed integer can hold @stable ICU 2.0 */
#   define INT8_MIN        ((int8_t)(-128))
#endif
#ifndef INT16_MIN
/** The smallest value a 16 bit signed integer can hold @stable ICU 2.0 */
#   define INT16_MIN       ((int16_t)(-32767-1))
#endif
#ifndef INT32_MIN
/** The smallest value a 32 bit signed integer can hold @stable ICU 2.0 */
#   define INT32_MIN       ((int32_t)(-2147483647-1))
#endif

#ifndef INT8_MAX
/** The largest value an 8 bit signed integer can hold @stable ICU 2.0 */
#   define INT8_MAX        ((int8_t)(127))
#endif
#ifndef INT16_MAX
/** The largest value a 16 bit signed integer can hold @stable ICU 2.0 */
#   define INT16_MAX       ((int16_t)(32767))
#endif
#ifndef INT32_MAX
/** The largest value a 32 bit signed integer can hold @stable ICU 2.0 */
#   define INT32_MAX       ((int32_t)(2147483647))
#endif

#ifndef UINT8_MAX
/** The largest value an 8 bit unsigned integer can hold @stable ICU 2.0 */
#   define UINT8_MAX       ((uint8_t)(255U))
#endif
#ifndef UINT16_MAX
/** The largest value a 16 bit unsigned integer can hold @stable ICU 2.0 */
#   define UINT16_MAX      ((uint16_t)(65535U))
#endif
#ifndef UINT32_MAX
/** The largest value a 32 bit unsigned integer can hold @stable ICU 2.0 */
#   define UINT32_MAX      ((uint32_t)(4294967295U))
#endif

#if defined(U_INT64_T_UNAVAILABLE)
# error int64_t is required for decimal format and rule-based number format.
#else
# ifndef INT64_C
/**
 * Provides a platform independent way to specify a signed 64-bit integer constant.
 * note: may be wrong for some 64 bit platforms - ensure your compiler provides INT64_C
 * @stable ICU 2.8
 */
#   define INT64_C(c) c ## LL
# endif
# ifndef UINT64_C
/**
 * Provides a platform independent way to specify an unsigned 64-bit integer constant.
 * note: may be wrong for some 64 bit platforms - ensure your compiler provides UINT64_C
 * @stable ICU 2.8
 */
#   define UINT64_C(c) c ## ULL
# endif
# ifndef U_INT64_MIN
/** The smallest value a 64 bit signed integer can hold @stable ICU 2.8 */
#     define U_INT64_MIN       ((int64_t)(INT64_C(-9223372036854775807)-1))
# endif
# ifndef U_INT64_MAX
/** The largest value a 64 bit signed integer can hold @stable ICU 2.8 */
#     define U_INT64_MAX       ((int64_t)(INT64_C(9223372036854775807)))
# endif
# ifndef U_UINT64_MAX
/** The largest value a 64 bit unsigned integer can hold @stable ICU 2.8 */
#     define U_UINT64_MAX      ((uint64_t)(UINT64_C(18446744073709551615)))
# endif
#endif

/*==========================================================================*/
/* Boolean data type                                                        */
/*==========================================================================*/

/** The ICU boolean type @stable ICU 2.0 */
typedef int8_t UBool;

#ifndef TRUE
/** The TRUE value of a UBool @stable ICU 2.0 */
#   define TRUE  1
#endif
#ifndef FALSE
/** The FALSE value of a UBool @stable ICU 2.0 */
#   define FALSE 0
#endif


/*==========================================================================*/
/* Unicode data types                                                       */
/*==========================================================================*/

/* wchar_t-related definitions -------------------------------------------- */

/*
 * \def U_WCHAR_IS_UTF16
 * Defined if wchar_t uses UTF-16.
 *
 * @stable ICU 2.0
 */
/*
 * \def U_WCHAR_IS_UTF32
 * Defined if wchar_t uses UTF-32.
 *
 * @stable ICU 2.0
 */
#if !defined(U_WCHAR_IS_UTF16) && !defined(U_WCHAR_IS_UTF32)
#   ifdef __STDC_ISO_10646__
#       if (U_SIZEOF_WCHAR_T==2)
#           define U_WCHAR_IS_UTF16
#       elif (U_SIZEOF_WCHAR_T==4)
#           define  U_WCHAR_IS_UTF32
#       endif
#   elif defined __UCS2__
#       if (U_PF_OS390 <= U_PLATFORM && U_PLATFORM <= U_PF_OS400) && (U_SIZEOF_WCHAR_T==2)
#           define U_WCHAR_IS_UTF16
#       endif
#   elif defined(__UCS4__) || (U_PLATFORM == U_PF_OS400 && defined(__UTF32__))
#       if (U_SIZEOF_WCHAR_T==4)
#           define U_WCHAR_IS_UTF32
#       endif
#   elif U_PLATFORM_IS_DARWIN_BASED || (U_SIZEOF_WCHAR_T==4 && U_PLATFORM_IS_LINUX_BASED)
#       define U_WCHAR_IS_UTF32
#   elif U_PLATFORM_HAS_WIN32_API
#       define U_WCHAR_IS_UTF16
#   endif
#endif

/* UChar and UChar32 definitions -------------------------------------------- */

/** Number of bytes in a UChar. @stable ICU 2.0 */
#define U_SIZEOF_UCHAR 2

/**
 * \var UChar
 * Define UChar to be UCHAR_TYPE, if that is #defined (for example, to char16_t),
 * or wchar_t if that is 16 bits wide; always assumed to be unsigned.
 * If neither is available, then define UChar to be uint16_t.
 *
 * This makes the definition of UChar platform-dependent
 * but allows direct string type compatibility with platforms with
 * 16-bit wchar_t types.
 *
 * @stable ICU 4.4
 */
#if defined(UCHAR_TYPE)
    typedef UCHAR_TYPE UChar;
/* Not #elif U_HAVE_CHAR16_T -- because that is type-incompatible with pre-C++11 callers
    typedef char16_t UChar;  */
#elif U_SIZEOF_WCHAR_T==2
    typedef wchar_t UChar;
#elif defined(__CHAR16_TYPE__)
    typedef __CHAR16_TYPE__ UChar;
#else
    typedef uint16_t UChar;
#endif

/**
 * Define UChar32 as a type for single Unicode code points.
 * UChar32 is a signed 32-bit integer (same as int32_t).
 *
 * The Unicode code point range is 0..0x10ffff.
 * All other values (negative or >=0x110000) are illegal as Unicode code points.
 * They may be used as sentinel values to indicate "done", "error"
 * or similar non-code point conditions.
 *
 * Before ICU 2.4 (Jitterbug 2146), UChar32 was defined
 * to be wchar_t if that is 32 bits wide (wchar_t may be signed or unsigned)
 * or else to be uint32_t.
 * That is, the definition of UChar32 was platform-dependent.
 *
 * @see U_SENTINEL
 * @stable ICU 2.4
 */
typedef int32_t UChar32;

/**
 * This value is intended for sentinel values for APIs that
 * (take or) return single code points (UChar32).
 * It is outside of the Unicode code point range 0..0x10ffff.
 * 
 * For example, a "done" or "error" value in a new API
 * could be indicated with U_SENTINEL.
 *
 * ICU APIs designed before ICU 2.4 usually define service-specific "done"
 * values, mostly 0xffff.
 * Those may need to be distinguished from
 * actual U+ffff text contents by calling functions like
 * CharacterIterator::hasNext() or UnicodeString::length().
 *
 * @return -1
 * @see UChar32
 * @stable ICU 2.4
 */
#define U_SENTINEL (-1)

#include "unicode/urename.h"

#endif
