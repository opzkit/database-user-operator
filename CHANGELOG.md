# Changelog

## [0.4.0](https://github.com/opzkit/database-user-operator/compare/v0.3.0...v0.4.0) (2026-05-22)


### Features

* **scaleway:** write secrets to Scaleway native path + name ([#170](https://github.com/opzkit/database-user-operator/issues/170)) ([2036e01](https://github.com/opzkit/database-user-operator/commit/2036e01f0c5d6b83a77ea760fd4a1e00752ba0b2))

## [0.3.0](https://github.com/opzkit/database-user-operator/compare/v0.2.0...v0.3.0) (2026-05-22)


### ⚠ BREAKING CHANGES

* pluggable secrets/connection backends (k8s, infisical, aws) ([#135](https://github.com/opzkit/database-user-operator/issues/135))

### Features

* pluggable secrets/connection backends (k8s, infisical, aws) ([#135](https://github.com/opzkit/database-user-operator/issues/135)) ([ccc44a8](https://github.com/opzkit/database-user-operator/commit/ccc44a851e906aa5afd1993228bd118c33b9b07a))
* **secrets:** add Scaleway Secret Manager backend ([#152](https://github.com/opzkit/database-user-operator/issues/152)) ([8b5abde](https://github.com/opzkit/database-user-operator/commit/8b5abde49a43b1670342319fc8b164cdb781c219))


### Bug Fixes

* **deps:** update module github.com/onsi/ginkgo/v2 to v2.29.0 ([#165](https://github.com/opzkit/database-user-operator/issues/165)) ([f7ebd67](https://github.com/opzkit/database-user-operator/commit/f7ebd67d9de2b9467360966c6390b3baba7ec8b2))
* **deps:** update module github.com/onsi/gomega to v1.41.0 ([#166](https://github.com/opzkit/database-user-operator/issues/166)) ([0089d95](https://github.com/opzkit/database-user-operator/commit/0089d958cdd1301900ceae6ac68259d701f01e27))
* **deps:** update module k8s.io/api to v0.36.1 ([#161](https://github.com/opzkit/database-user-operator/issues/161)) ([5a17909](https://github.com/opzkit/database-user-operator/commit/5a1790989af6c43cc47a4324da943b1cb738e8b3))
* **deps:** update module k8s.io/apimachinery to v0.36.1 ([#160](https://github.com/opzkit/database-user-operator/issues/160)) ([9db7efa](https://github.com/opzkit/database-user-operator/commit/9db7efa282ae7a15043375e3358a917ea622dcca))
* **deps:** update module k8s.io/client-go to v0.36.1 ([#162](https://github.com/opzkit/database-user-operator/issues/162)) ([1232c17](https://github.com/opzkit/database-user-operator/commit/1232c170aa5ea6ffa13fc5e104bc99881d2f80a0))
* **deps:** update module sigs.k8s.io/controller-runtime to v0.24.1 ([#158](https://github.com/opzkit/database-user-operator/issues/158)) ([1c4e481](https://github.com/opzkit/database-user-operator/commit/1c4e481e85dea1c3062184138942345251d637ef))

## [0.2.0](https://github.com/opzkit/database-user-operator/compare/v0.1.1...v0.2.0) (2026-05-07)


### Features

* stamp version/gitCommit/buildDate into manager binary ([#147](https://github.com/opzkit/database-user-operator/issues/147)) ([30d873e](https://github.com/opzkit/database-user-operator/commit/30d873ef35089910ce3ea698936fb6dec577aef5))


### Bug Fixes

* **ci:** pin setup-envtest to release-0.23 branch ([#138](https://github.com/opzkit/database-user-operator/issues/138)) ([c4e8dcd](https://github.com/opzkit/database-user-operator/commit/c4e8dcd22d019ada54568b48e02dece32f566351))
* **deps:** update aws-sdk-go-v2 monorepo ([#14](https://github.com/opzkit/database-user-operator/issues/14)) ([2d8913e](https://github.com/opzkit/database-user-operator/commit/2d8913e24bf2064fc606f5b6c2303ba498bd6f9d))
* **deps:** update aws-sdk-go-v2 monorepo ([#78](https://github.com/opzkit/database-user-operator/issues/78)) ([2a8f998](https://github.com/opzkit/database-user-operator/commit/2a8f998c6208511c677677038ad16bada1be2e99))
* **deps:** update aws-sdk-go-v2 monorepo ([#90](https://github.com/opzkit/database-user-operator/issues/90)) ([2354579](https://github.com/opzkit/database-user-operator/commit/23545791cdc16147d0d4156b511bb5ff9e5d0b5b))
* **deps:** update kubernetes packages to v0.35.0 ([#34](https://github.com/opzkit/database-user-operator/issues/34)) ([651909d](https://github.com/opzkit/database-user-operator/commit/651909d95a05571fc790246c695063cbd7fae00a))
* **deps:** update kubernetes packages to v0.35.2 ([#68](https://github.com/opzkit/database-user-operator/issues/68)) ([e00430c](https://github.com/opzkit/database-user-operator/commit/e00430c0cfad587005e17a8683c782a128121036))
* **deps:** update module github.com/aws/aws-sdk-go-v2/config to v1.32.8 ([#77](https://github.com/opzkit/database-user-operator/issues/77)) ([87cf07a](https://github.com/opzkit/database-user-operator/commit/87cf07a59adab35bb383073f265ef0adbd2470c4))
* **deps:** update module github.com/go-sql-driver/mysql to v1.10.0 ([#133](https://github.com/opzkit/database-user-operator/issues/133)) ([a9e3dbd](https://github.com/opzkit/database-user-operator/commit/a9e3dbd31e094b372b9df7b1e647f71ebf6289c2))
* **deps:** update module github.com/lib/pq to v1.11.2 ([#59](https://github.com/opzkit/database-user-operator/issues/59)) ([388d518](https://github.com/opzkit/database-user-operator/commit/388d5186b73f95a7d238c29d460f9815ce4d189d))
* **deps:** update module github.com/lib/pq to v1.12.3 ([#109](https://github.com/opzkit/database-user-operator/issues/109)) ([d5d5c33](https://github.com/opzkit/database-user-operator/commit/d5d5c332864dc0c7231ebee16200b0616666a18c))
* **deps:** update module github.com/onsi/ginkgo/v2 to v2.28.1 ([#31](https://github.com/opzkit/database-user-operator/issues/31)) ([099a361](https://github.com/opzkit/database-user-operator/commit/099a36195623b2aec400d3d032ab5f591aace1f7))
* **deps:** update module github.com/onsi/ginkgo/v2 to v2.28.3 ([#131](https://github.com/opzkit/database-user-operator/issues/131)) ([3be2009](https://github.com/opzkit/database-user-operator/commit/3be2009f45b2c20c3b225161e4e57f4f095ccc57))
* **deps:** update module github.com/onsi/gomega to v1.39.1 ([#32](https://github.com/opzkit/database-user-operator/issues/32)) ([6bcc403](https://github.com/opzkit/database-user-operator/commit/6bcc4036aef4b002cdb817e18bc2b34eac772d9f))
* **deps:** update module github.com/onsi/gomega to v1.40.0 ([#132](https://github.com/opzkit/database-user-operator/issues/132)) ([09d63fe](https://github.com/opzkit/database-user-operator/commit/09d63feac3d0dbbc5a6f8814d80905042d193e5a))
* **deps:** update module sigs.k8s.io/controller-runtime to v0.23.1 ([#52](https://github.com/opzkit/database-user-operator/issues/52)) ([ea0eae9](https://github.com/opzkit/database-user-operator/commit/ea0eae9c5f9050c5ecffceed3f034bb7b54a6338))
* **deps:** update module sigs.k8s.io/controller-runtime to v0.24.0 ([#94](https://github.com/opzkit/database-user-operator/issues/94)) ([db2e202](https://github.com/opzkit/database-user-operator/commit/db2e20211a6b77a4bc928d1be9c2205acd5f06ba))
* keep gomega in go.sum across renovate updates ([#149](https://github.com/opzkit/database-user-operator/issues/149)) ([d29f3d3](https://github.com/opzkit/database-user-operator/commit/d29f3d3a43a86ea1908f0514553f23e2209d20fd))
* use go-version-file instead of hardcoded version ([#86](https://github.com/opzkit/database-user-operator/issues/86)) ([1cd77de](https://github.com/opzkit/database-user-operator/commit/1cd77dea853b4afafa2215c03828351e572868c9))

## [0.1.1](https://github.com/opzkit/database-user-operator/compare/v0.1.0...v0.1.1) (2025-11-14)


### Bug Fixes

* create db managed ([#6](https://github.com/opzkit/database-user-operator/issues/6)) ([82834de](https://github.com/opzkit/database-user-operator/commit/82834de9f6e01254eab52720a02da545e3957eae))

## [0.1.0](https://github.com/opzkit/database-user-operator/compare/v0.1.0...v0.1.0) (2025-11-13)


### Miscellaneous Chores

* release 0.1.0 ([337bed4](https://github.com/opzkit/database-user-operator/commit/337bed416873c2454060632c8669461eb65976c6))
* release 0.1.0 ([09312b5](https://github.com/opzkit/database-user-operator/commit/09312b57c33027e659521500ab75cdc06719d968))

## 0.1.0 (2025-11-13)


### Miscellaneous Chores

* release 0.1.0 ([09312b5](https://github.com/opzkit/database-user-operator/commit/09312b57c33027e659521500ab75cdc06719d968))
