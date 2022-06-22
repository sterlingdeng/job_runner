package bufferz

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_MultiReader_BasicBehavior(t *testing.T) {
	multireader := NewMultiReaderBuffer()
	input := []byte("foo.bar.baz")
	n, err := multireader.Write(input)
	require.NoError(t, err)
	require.Equal(t, len(input), n)

	reader := multireader.GetReader(context.Background())
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, input, got)
	err = multireader.Close()
	require.NoError(t, err)
	n, err = reader.Read(make([]byte, 1))
	require.Equal(t, 0, n)
	require.Equal(t, err, io.EOF)
}

func Test_MultiReader_ReadSliceIsLargerThanInternalBuffer(t *testing.T) {
	multireader := NewMultiReaderBuffer()
	input := []byte("foo.bar.baz")
	n, err := multireader.Write(input)
	require.NoError(t, err)
	require.Equal(t, len(input), n)

	reader := multireader.GetReader(context.Background())
	var got []byte
	// assert that we try to fill up read buffer even though the len is bigger than the data
	readbuf := make([]byte, 1024*32)
	n, err = reader.Read(readbuf)
	require.NoError(t, err)
	require.Equal(t, len(input), n)
	got = append(got, readbuf...)
	require.Equal(t, input, got[:len(input)])
}

func Test_MultiReader_ReaderBlocksWaitingForWrites(t *testing.T) {
	// assert the readers will block when waiting for writes so the loop doesn't spin
	multireader := NewMultiReaderBuffer()

	reader := multireader.GetReader(context.Background())

	errch := make(chan error)
	go func() {
		_, err := reader.Read([]byte{})
		errch <- err
	}()

	time.Sleep(300 * time.Millisecond)
	_, err := multireader.Write([]byte("teleport"))
	require.NoError(t, err)
	<-errch
	multireader.Close()
}

func Test_MultiReader_CallingCloseWhenReaderIsAtEnd(t *testing.T) {
	multireader := NewMultiReaderBuffer()
	input := []byte("foo.bar.baz")
	n, err := multireader.Write(input)
	require.NoError(t, err)
	require.Equal(t, len(input), n)

	reader := multireader.GetReader(context.Background())

	_, err = reader.Read(make([]byte, len(input)))
	require.NoError(t, err)

	go func() {
		multireader.Close()
	}()

	// reader will block until multireader.Close() is called
	n, err = reader.Read(make([]byte, len(input)))
	require.Equal(t, 0, n)
	require.Equal(t, err, io.EOF)
}

func Test_MultiReader_ReadDataAfterClose(t *testing.T) {
	multireader := NewMultiReaderBuffer()
	input := []byte("foo.bar.baz")
	_, _ = multireader.Write(input)

	err := multireader.Close()
	require.NoError(t, err)

	reader := multireader.GetReader(context.Background())
	got, err := io.ReadAll(reader)
	require.Equal(t, string(input), string(got))
	require.NoError(t, err)
}

func Test_MultiReader_CancelReaderWhileWaitingForMoreData(t *testing.T) {
	multireader := NewMultiReaderBuffer()
	input := []byte("foo.bar.baz")
	_, _ = multireader.Write(input)

	deadline := time.Now().Add(50 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	reader := multireader.GetReader(ctx)

	// read the entire input
	buf := make([]byte, len(input))
	_, err := reader.Read(buf)
	require.NoError(t, err)
	// the next read call must complete and not block and should complete at around the deadline
	_, err = reader.Read(buf)
	require.Equal(t, context.DeadlineExceeded, err)
	require.WithinDuration(t, deadline, time.Now(), 20*time.Millisecond)
}

func Test_MultiReader_CancelOneReaderOfManyWhileWaitingForMoreData(t *testing.T) {
	multireader := NewMultiReaderBuffer()
	input := []byte("foo.bar.baz")
	_, _ = multireader.Write(input)

	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := io.ReadAll(multireader.GetReader(ctx))
			require.NoError(t, err)
		}(i)
	}

	deadline := time.Now().Add(80 * time.Millisecond)
	ctxDeadline, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	readerWithDeadline := multireader.GetReader(ctxDeadline)

	// read the entire input
	_, err := io.ReadAll(readerWithDeadline)
	require.Equal(t, context.DeadlineExceeded, err)

	err = multireader.Close()
	require.NoError(t, err)
	wg.Wait()
}
