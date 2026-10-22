# Changelog

## [0.2.0](https://github.com/Yornik/go-practice-web/compare/v0.1.1...v0.2.0) (2026-10-22)


### Features

* migrate stored slugs and certificate kinds to the course of 50 ([6c5afbd](https://github.com/Yornik/go-practice-web/commit/6c5afbddc0932b4159b7b70561819f779fd4027a))
* read certificates through the course renames and fit 10 tiers in the PDF ([2892324](https://github.com/Yornik/go-practice-web/commit/28923240455c1dabac80e18d3155db9ea68358a9))
* read tier names and renamed exercise slugs from the course ([68d1ecb](https://github.com/Yornik/go-practice-web/commit/68d1ecbfe40662729f230b12b38c5437eec5db48))
* serve the course of 50 exercises in 10 tiers ([684f310](https://github.com/Yornik/go-practice-web/commit/684f31076c96aeb26a912b5901d3e4059dd522c5))
* serve the course of 50 with old links, earlier certificates and course counts ([3f6f208](https://github.com/Yornik/go-practice-web/commit/3f6f20880383b5cd852b607150b80b1bdcfec86e))


### Bug Fixes

* back up the database before a migration and redirect old slugs with 302 ([a812d4b](https://github.com/Yornik/go-practice-web/commit/a812d4bd023723ab6dec469724ae7e817eda2a52))
* move code kept under an old slug to the renamed exercise ([4ec83e6](https://github.com/Yornik/go-practice-web/commit/4ec83e677ec2e7a78929e1d50818cb40526924b7))
* point explanations at the first exercise that teaches the concept ([52198e9](https://github.com/Yornik/go-practice-web/commit/52198e9ff333c123318c55e0216452f893a65938))
* say what a certificate of the first course was issued as ([5cb32b1](https://github.com/Yornik/go-practice-web/commit/5cb32b1f7eff6d9ad07af6bc29d6642664d6c9c4))
* serve the course with the 12-linefilter and 29-grep fixes ([f74d9a6](https://github.com/Yornik/go-practice-web/commit/f74d9a671cef65efb4425634e82f5f9e958cc0f4))
* serve the course with the 12-linefilter and 29-grep fixes ([511aa8d](https://github.com/Yornik/go-practice-web/commit/511aa8dc0f5731c4fbd92bfcc9df305b5f66659d))
* tell learners the runner is updating when it lacks an exercise ([61fc023](https://github.com/Yornik/go-practice-web/commit/61fc023c616d9cedb9fee1d05062297e9cf42042))

## [0.1.1](https://github.com/Yornik/go-practice-web/compare/v0.1.0...v0.1.1) (2026-10-21)


### Bug Fixes

* keep site CSS out of the editor's suggest list and find widget ([ae60a52](https://github.com/Yornik/go-practice-web/commit/ae60a5256b4920ff0562959c60ade75f635c832a))
* keep site CSS out of the editor's suggest list and find widget ([c560f73](https://github.com/Yornik/go-practice-web/commit/c560f730b943f65dab494bb3dbf4ac3fce182670))
* rename the remaining site classes that Monaco also uses ([1b1c1fc](https://github.com/Yornik/go-practice-web/commit/1b1c1fca687f0139183c617806097148fc0d0431))

## 0.1.0 (2026-10-17)


### Features

* Go Practice web IDE with passkey accounts, a sandboxed runner and certificates ([6dfc87e](https://github.com/Yornik/go-practice-web/commit/6dfc87ea31ba423df864fbd466ac6fd99238f303))
* release with Release Please ([5f0e598](https://github.com/Yornik/go-practice-web/commit/5f0e598ee68736be51ac1e10dcbe4d3a507caaa6))
* require a verified email before the course ([a3c4744](https://github.com/Yornik/go-practice-web/commit/a3c47441d4eda81ebd935468bbf8f610299af524))
