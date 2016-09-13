// Copyright (c) 2016, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

package org.rocksdb;

import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Offers functionality for implementations of
 * {@link AbstractNativeReference} which have an immutable reference to the
 * underlying native C++ object
 */
public abstract class AbstractImmutableNativeReference
    extends AbstractNativeReference {

  /**
   * A flag indicating whether the current {@code AbstractNativeReference} is
   * responsible to free the underlying C++ object
   */
  private final AtomicBoolean owningHandle_;

  protected AbstractImmutableNativeReference(final boolean owningHandle) {
    this.owningHandle_ = new AtomicBoolean(owningHandle);
  }

  @Override
  public boolean isOwningHandle() {
    return owningHandle_.get();
  }

  /**
   * Releases this {@code AbstractNativeReference} from  the responsibility of
   * freeing the underlying native C++ object
   * <p>
   * This will prevent the object from attempting to delete the underlying
   * native object in its finalizer. This must be used when another object
   * takes over ownership of the native object or both will attempt to delete
   * the underlying object when garbage collected.
   * <p>
   * When {@code disOwnNativeHandle()} is called, {@code dispose()} will
   * subsequently take no action. As a result, incorrect use of this function
   * may cause a memory leak.
   * </p>
   *
   * @see #dispose()
   */
  protected final void disOwnNativeHandle() {
    owningHandle_.set(false);
  }

  @Override
  public void close() {
    if (owningHandle_.compareAndSet(true, false)) {
      disposeInternal();
    }
  }

  /**
   * The helper function of {@link AbstractImmutableNativeReference#dispose()}
   * which all subclasses of {@code AbstractImmutableNativeReference} must
   * implement to release their underlying native C++ objects.
   */
  protected abstract void disposeInternal();
}
