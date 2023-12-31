// Copyright 2022 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package multitenant

import (
	"context"
	"io"

	"github.com/cockroachdb/cockroach/pkg/cloud"
	"github.com/cockroachdb/cockroach/pkg/settings"
	"github.com/cockroachdb/cockroach/pkg/util/ioctx"
)

// DefaultBytesAllowedBeforeAccounting are how many bytes we will read/written
// before trying to wait for RUs. The goal here is to avoid waiting in loops for
// in common case without allowing an unbounded number of bytes read/written
// before accounting for them.
var DefaultBytesAllowedBeforeAccounting = settings.RegisterIntSetting(
	settings.TenantReadOnly,
	"tenant_external_io_default_bytes_allowed_before_accounting",
	"controls how many bytes will be read/written before blocking for RUs when writing to external storage in a tenant",
	16<<20, // 16 MB
	settings.PositiveInt,
)

// readWriteAccounter is cloud.ReadWriteInterceptor that records ingress and egress bytes.
type readWriteAccounter struct {
	recorder TenantSideExternalIORecorder
	limit    int64
}

// NewReadWriteAccounter returns a cloud.ExternalStorage
// that records ingress and egress bytes iff the storage requires
// external accounting.
//
// Ingress and egress bytes are recorded once at least limit bytes
// have been read or written.
func NewReadWriteAccounter(
	recorder TenantSideExternalIORecorder, limit int64,
) cloud.ReadWriterInterceptor {
	if recorder == nil {
		return nil
	}
	return &readWriteAccounter{
		limit:    limit,
		recorder: recorder,
	}
}

func (a *readWriteAccounter) Writer(
	ctx context.Context, s cloud.ExternalStorage, w io.WriteCloser,
) io.WriteCloser {
	if !s.RequiresExternalIOAccounting() {
		return w
	}
	return &accountingWriter{
		ctx:      ctx,
		inner:    w,
		limit:    a.limit,
		recorder: a.recorder,
	}
}

func (a *readWriteAccounter) Reader(
	_ context.Context, s cloud.ExternalStorage, r ioctx.ReadCloserCtx,
) ioctx.ReadCloserCtx {
	if !s.RequiresExternalIOAccounting() {
		return r
	}
	return &accountingReader{
		inner:    r,
		limit:    a.limit,
		recorder: a.recorder,
	}
}

// accountingWriter is an io.WriteCloser that tracks how many total bytes have been written. If limit is > 0, then the
// writer will record the written bytes and wait for the associated RUs in a Write call if more than limit bytes have been
// written. On Close, any previously unaccounted for RUs will be recorded.
//
// If limit <= 0 then we will wait for RUs only on Close().
//
// NB: The implementation allows roughly 2x the limit to be unaccounted if the caller is making write calls with a large
// values just under the limit.
type accountingWriter struct {
	ctx      context.Context
	inner    io.WriteCloser
	recorder TenantSideExternalIORecorder
	limit    int64

	count int64
}

var _ io.WriteCloser = (*accountingWriter)(nil)

func (aw *accountingWriter) Write(d []byte) (int, error) {
	// If past writes have pushed us past the limit, account for them before allowing this write.
	if err := aw.maybeWaitForRUs(); err != nil {
		return 0, err
	}

	// If this single write is larger than the limit, immediately account for it.
	if int64(len(d)) > aw.limit {
		return aw.immediatelyAccountedWrite(d)
	}

	n, err := aw.inner.Write(d)
	aw.count += int64(n)
	return n, err
}

func (aw *accountingWriter) immediatelyAccountedWrite(d []byte) (int, error) {
	writeLen := int64(len(d))
	if err := aw.recorder.ExternalIOWriteWait(aw.ctx, writeLen); err != nil {
		return 0, err
	}
	n, err := aw.inner.Write(d)
	if err != nil {
		aw.recorder.ExternalIOWriteFailure(aw.ctx, int64(n), writeLen-int64(n))
		return n, err
	}
	aw.recorder.ExternalIOWriteSuccess(aw.ctx, int64(n))
	return n, err
}

// Close closes the underlying Writer and also waits for any RUs that weren't
// accounted for on a previous call to Write.
func (aw *accountingWriter) Close() error {
	// NB: We only record bytes actually written (according to the underlying
	// writer) in aw.count.
	if err := aw.recorder.ExternalIOWriteWait(aw.ctx, aw.count); err != nil {
		// We still want to close the underlying writer.
		_ = aw.inner.Close()
		return err
	}
	aw.recorder.ExternalIOWriteSuccess(aw.ctx, aw.count)
	aw.count = 0
	return aw.inner.Close()
}

func (aw *accountingWriter) maybeWaitForRUs() error {
	if aw.limit > 0 && aw.count >= aw.limit {
		if err := aw.recorder.ExternalIOWriteWait(aw.ctx, aw.count); err != nil {
			return err
		}
		aw.recorder.ExternalIOWriteSuccess(aw.ctx, aw.count)
		aw.count = 0
	}
	return nil
}

// accountingReader is an ioctx.ReadCloser that tracks how many total bytes have
// been read. If limit is > 0, then the reader will record the read bytes and
// wait for the associated RUs in a Read call if more than limit bytes have been
// read. On Close, any previously unaccounted for RUs will be recorded.
//
// If limit <= 0 then we will wait for RUs only on Close().
type accountingReader struct {
	inner    ioctx.ReadCloserCtx
	recorder TenantSideExternalIORecorder
	limit    int64

	count int64
}

var _ ioctx.ReadCloserCtx = (*accountingReader)(nil)

// Read implements ioctx.ReadCloserCtx.
func (ar *accountingReader) Read(ctx context.Context, d []byte) (int, error) {
	// If past reads have pushed us past the limit, account for them before
	// allowing this read.
	if ar.limit > 0 && ar.count >= ar.limit {
		if err := ar.recorder.ExternalIOReadWait(ctx, ar.count); err != nil {
			return 0, err
		}
		ar.count = 0
	}

	n, err := ar.inner.Read(ctx, d)
	ar.count += int64(n)
	return n, err
}

// Close implements ioctx.ReadCloserCtx.
func (ar *accountingReader) Close(ctx context.Context) error {
	if err := ar.recorder.ExternalIOReadWait(ctx, ar.count); err != nil {
		_ = ar.inner.Close(ctx)
		return err
	}
	ar.count = 0
	return ar.inner.Close(ctx)
}
