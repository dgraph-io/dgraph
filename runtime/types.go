package runtime

type Version struct {
	Spec_name         []byte
	Impl_name         []byte
	Authoring_version int32
	Spec_version      int32
	Impl_version      int32
}
