# ffmpeg and ffprobe

- **Version:** 9.0.1
- **Built by:** Martin Riedl, <https://ffmpeg.martin-riedl.de>
- **Licence:** GNU General Public License, version 3 (`--enable-gpl
  --enable-version3`). The full text is Lasso's own [LICENSE](../../LICENSE),
  which is the same licence.
- **Copyright:** © 2000–2026 the FFmpeg developers, and the authors of each
  library below.

These are static builds: the libraries below are compiled into the two
programs rather than shipped beside them.

openssl, fontconfig, xml2, freetype, harfbuzz, bluray, snappy, srt, vmaf, ass,
klvanc, zimg, zvbi, aom, dav1d, openh264, openjpeg, rav1e, svtav1, vpx, vvenc,
webp, x264, x265, mp3lame, opus, vorbis, theora

## The source code

The GPL gives everyone who receives these programs the right to their
complete source code. It is here:

- FFmpeg 9.0.1: <https://ffmpeg.org/releases/ffmpeg-9.0.1.tar.xz>
- The script that built these binaries, with the version of every library it
  used: <https://git.martin-riedl.de/ffmpeg/build-script>
- Each library's own source, linked from <https://ffmpeg.martin-riedl.de>.

If any of these stops being available, open an issue at
<https://github.com/swayyaam/lasso/issues> and the source for the version
Lasso shipped will be provided.

## Build configuration

As the binaries report it (`ffmpeg -version`):

```
--prefix=/Volumes/ffmpeg_arm64/out --pkg-config-flags=--static
--extra-version='https://www.martin-riedl.de' --enable-gray --enable-libxml2
--enable-version3 --enable-gpl --enable-openssl --enable-libfreetype
--enable-fontconfig --enable-libharfbuzz --enable-libbluray --enable-libsnappy
--enable-libsrt --enable-libvmaf --enable-libass --enable-libklvanc
--enable-libzimg --enable-libzvbi --enable-libaom --enable-libdav1d
--enable-libopenh264 --enable-libopenjpeg --enable-librav1e --enable-libsvtav1
--enable-libvpx --enable-libvvenc --enable-libwebp --enable-libx264
--enable-libx265 --enable-libmp3lame --enable-libopus --enable-libvorbis
--enable-libtheora
```
