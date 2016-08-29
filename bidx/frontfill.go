package bidx

// Frontfill updates the indices as mutations come in.
//func (s *Indices) Frontfill() error {
//	for _, index := range s.Index {
//		go index.Backfill(ps, s.Done)
//	}
//	for i := 0; i < len(s.Index); i++ {
//		if err := <-s.Done; err != nil {
//			return err
//		}
//	}
//	return nil
//}
