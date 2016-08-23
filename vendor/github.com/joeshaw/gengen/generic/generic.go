// package generic defines generic types the gengen tool replaces with
// provided, more specific types.  Each of the T, U, and V types are
// optional (although usage doesn't make much sense without at least
// T)
package generic

// T is the first type substituted by the gengen tool
type T interface{}

// U is the second type substituted by the gengen tool
type U interface{}

// V is the third type substituted by the gengen tool
type V interface{}
