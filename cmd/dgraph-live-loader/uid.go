package main

func xidKey(xid string) string {
	// Prefix to avoid key clashes with other data stored in badger.
	return "\x01" + xid
}

func (l *loader) NodeBlank(varname string) (uint64, error) {
	if len(varname) == 0 {
		uid, err := d.alloc.AllocateUid()
		if err != nil {
			return 0, err
		}
		return uid, nil
	}
	uid, _, err := d.alloc.AssignUid(xidKey("_:" + varname))
	return uid, err
}

// TODO - This should come from server.
func (l *loader) NodeXid(xid string, storeXid bool) (uint64, error) {
	if len(xid) == 0 {
		return 0, ErrEmptyXid
	}
	uid, _, err := d.alloc.AssignUid(xidKey(xid))
	//	if storeXid && isNew {
	//		e := n.Edge("xid")
	//		x.Check(e.SetValueString(xid))
	//		d.BatchSet(e)
	//	}
	return uid, nil
}
