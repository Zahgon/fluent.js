# Project Fluent

Fluent.js is a JavaScript implementation of Project Fluent, a localization
framework designed to unleash the expressive power of the natural language.

Project Fluent keeps simple things simple and makes complex things possible.
The syntax used for describing translations is easy to read and understand. At
the same time it allows, when necessary, to represent complex concepts from
natural languages like gender, plurals, conjugations, and others.

## Packages

This repository is a Go port of Fluent.js at commit 9a925d2a. Its module, `github.com/projectfluent/fluent-go`,
has these packages:

- `syntax`: the FTL parser, AST, serializer, `Visitor` and `Transformer` (`@fluent/syntax`)
- `bundle`: `FluentBundle`, `FluentResource` and the runtime types (`@fluent/bundle`)
- `langneg`: language negotiation (`@fluent/langneg`)
- `sequence`: `MapBundleSync` (`@fluent/sequence`)
- `dedent`: `Ftl` (`@fluent/dedent`)

`internal/intl` reproduces the `Intl` objects those packages rely on, as Node.js 24.22.1 implements them, over the ICU
and CLDR data in `internal/icudata`. See `THIRD_PARTY_NOTICES.md`.

## Learn the FTL syntax

FTL is a localization file format used for describing translation resources.
FTL stands for _Fluent Translation List_.

FTL is designed to be simple to read, but at the same time allows to represent
complex concepts from natural languages like gender, plurals, conjugations,
and others.

    hello-user = Hello, { $username }!

[Read the Fluent Syntax Guide][] in order to learn more about the syntax. If
you're a tool author you may be interested in the formal [EBNF grammar][].

[read the fluent syntax guide]: https://projectfluent.org/fluent/guide/
[ebnf grammar]: https://github.com/projectfluent/fluent/tree/master/spec

## Discuss

We'd love to hear your thoughts on Project Fluent! Whether you're a localizer looking
for a better way to express yourself in your language, or a developer trying to
make your app localizable and multilingual, or a hacker looking for a project
to contribute to, please do get in touch!

- Discourse: https://discourse.mozilla.org/c/fluent

## Get Involved

Fluent.js is open-source, licensed under the Apache License, Version 2.0. We
encourage everyone to take a look at our code and we'll listen to your
feedback.

## Local Development

Build and test with `go build ./...` and `go test ./...`.
