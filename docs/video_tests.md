# Video tests

The Tast video tests are a set of tests used to validate various video decoder
and encoder implementations. A wide range of test scenarios are available. Check
the [tast video folder](https://chromium.googlesource.com/chromiumos/platform/tast-tests/+/refs/heads/master/src/chromiumos/tast/local/bundles/cros/video/)
for a list of all available tests.

[TOC]

## Capability check tests:
This test checks whether the device reports the correct set of capabilities (e.g. vp9 support).
To run this test use:

    tast run $HOST video.Capability*

##  Video Decoder tests:
Test video decoding functionality using the video_decode_accelerator_tests. These tests are implemented directly on top of the video decoder implementations. Various scenarios are tested. All decoded frames are compared to their expected checksums. Tests are available for various codecs such as h264, vp8 and vp9. Additionally tests are available using videos that change resolution during plaback. For more information about these tests check the [documentation](https://chromium.googlesource.com/chromium/src/+/lkgr/docs/media/gpu/video_decoder_test_usage.md). To run all tests use:

    tast run $HOST video.DecodeAccelH264* video.DecodeAccelVP*

There are variants of these tests with 'VD' in their names that test the new video decoder implementations, which are set to replace the current ones. To run all VD video decoder tests run:

    tast run $HOST video.DecodeAccelVD*

## Video Decoder Performance Tests
These tests measure video decode performance by running the video_decode_accelerator_perf_tests. These tests are implemented directly on top of the video decoder implementations. Various metrics are collected such as FPS, CPU usage and decode latency. Tests are available for various codecs using 1080p and 2160p videos, both in 30 and 60fps variants. For more information about these tests check the [documentation](https://chromium.googlesource.com/chromium/src/+/lkgr/docs/media/gpu/video_decoder_perf_test_usage.md) To run all tests use:

    tast run $HOST video.DecodeAccelPerf*

There are variants of these tests with 'VD' in their names that test the new video decoder implementations, which are set to replace the current ones. To run all VD video decoder tests run:

    tast run $HOST video.DecodeAccelVDPerf*

## Video decoder sanity checks:
These tests use the video_decode_accelerator_tests to decode a video stream with unsupported features. The tests verify whether a decoder is able to handle unexpected errors gracefully.

    tast run $HOST video.DecodeAccelSanity*

## Video encoder tests
These tests run the video_encode_accelerator_unittest to test encoding raw video frames. Tests are available that test encoding videos in various codecs and resolutions.
To run all video encode tests use:

    tast run $HOST video.EncodeAccelH264* video.EncodeAccelVP*

## Video encoder performance tests
These tests measure video encode performance by running the video_encode_accelerator_unittest. These tests are implemented directly on top of the video encoder implementations. Various metrics are collected such as CPU usage. Tests are available for various codecs and resolutions. To run all tests use:

    tast run $HOST video.EncodeAccelPerf*

## Video play tests
The video play tests verify whether video playback works by playing a video in the Chrome browser. The tests exercise the full Chrome stack, as opposed to the video decoder tests that only check the actual video decoder implementations. The tests check whether video playback works by any means possible: fallback on a software video decoder is allowed if hardware video decoding fails. Tests are available for h264, vp8 and vp9 videos. To run these tests use:

    tast run $HOST video.PlayH264* video.PlayVP*

The video play decode acceleration tests are similar to the normal video play tests. The difference is that these tests will only pass if hardware video decoding was successful, and don't allow fallback on a software video decoder. Tests are available for h264, vp8 and vp9, both for normal videos and videos using MSE. To run these tests use:

    tast run $HOST video.PlayDecodeAccelUsedH264* video.PlayDecodeAccelUsedVP* video.PlayDecodeAccelUsedMSE*

There are variants of these tests with 'VD' in their names that test the new video decoder implementations, which are set to replace the current ones. To run all VD video play tests run:

    tast run $HOST video.PlayVD* video.PlayDecodeAccelUsedVD*

## Video playback performance tests
The video playback performance tests measure video decoder performance by playing a video in the Chrome browser. The tests exercise the full Chrome stack, as opposed to the video decoder performance tests [add link] that only check the actual video decoder implementations.
Various metrics are collected such as CPU usage and the number of dropped frames. Tests are available for various codecs and resolutions, both in 30 and 60fps variants. To run all tests use:

    tast run $HOST video.PlaybackPerfAV1** video.PlaybackPerfh264* video.PlaybackPerfVP*

There are variants of these tests with 'VD' in their names that test the new video decoder implementations, which are set to replace the current ones. To run all VD video playback performance tests run:

    tast run $HOST video.PlaybackVDPerf*

## Video Seek tests:
These tests check whether seeking in a video works as expected, by playing a video in the Chrome browser while rapidly jumping between different points in the video. Tests are available for h264, VP8 and VP9 videos. In addition there are also tests present that verify seeking in resolution-changing videos. To run all video seek tests run:

    tast run $HOST video.Seek*

