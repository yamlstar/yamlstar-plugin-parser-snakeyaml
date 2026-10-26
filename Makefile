M ?= .cache/makes
MAKES-COMMIT := 0a4d1e3f63afe1cd36eacb0babec7bcfebdbe494
$(shell test -d $M || { \
  git clone -q https://github.com/makeplus/makes $M && \
  git -C $M checkout -q $(MAKES-COMMIT); \
})
GRAALVM-VERSION := 25
include $M/init.mk
ifeq ($(OS-NAME)-$(ARCH-NAME),macos-int64)
include $M/java.mk
else
include $M/graalvm.mk
endif
include $M/maven.mk
include $M/go.mk
include $M/perl.mk
include $M/yamlscript.mk
include $M/python.mk
include $M/gh.mk
include $M/shellcheck.mk
include $M/shell.mk

VERSION := 0.1.0
ARTIFACT := yamlstar-plugin-parser-snakeyaml
ENGINE := $(MAVEN-REPOSITORY)/org/yamlstar/snakeyaml-engine/3.2.0/\
  snakeyaml-engine-3.2.0.jar
ENGINE-JAR := $(subst $(space),,$(ENGINE))
MVN = $(MAVEN) --batch-mode --no-transfer-progress \
  -Dmaven.repo.local=$(MAVEN-REPOSITORY)
JAVA-SOURCES := $(wildcard src/main/java/org/yamlstar/snakeyaml/*.java)
NATIVE-SOURCES := $(wildcard src/native/java/org/yamlstar/snakeyaml/*.java)
LIB := lib/lib$(ARTIFACT).$(SO)
RELEASE-ARCH := $(if $(IS-INTEL),x64,$(if $(IS-LINUX),aarch64,arm64))
RELEASE-PLATFORM := $(OS-NAME)-$(RELEASE-ARCH)
RELEASE-NAME := $(ARTIFACT)-$(VERSION)-$(RELEASE-PLATFORM)
ARCHIVE := dist/$(RELEASE-NAME).tar.xz

default:: build

build: $(LIB)

build-java: $(MAVEN)
	$(MVN) package -DskipTests

test-java: $(MAVEN)
	$(MVN) test

install: $(MAVEN)
	$(MVN) install -DskipTests

target/native/.compiled: $(JAVA-SOURCES) $(NATIVE-SOURCES) pom.xml $(MAVEN)
	$(MVN) compile -Pnative
	mkdir -p target/native
	touch $@

$(LIB): Makefile target/native/.compiled native/shim.c plugin.edn \
  include/yamlstar_plugin.h $(GRAALVM)
	@case $(RELEASE-PLATFORM) in \
	  linux-x64|linux-aarch64|macos-arm64) ;; \
	  *) echo 'This platform supports the JVM artifact only' >&2; exit 1 ;; \
	esac
	mkdir -p lib target/native
	$(CC) -std=c11 -Wall -Wextra -Werror -fPIC -pthread -Iinclude \
	  -c native/shim.c -o target/native/shim.o
	$(GRAALVM) --shared --no-fallback -O1 -march=compatibility \
	  -cp target/native-build/classes:$(ENGINE-JAR) \
	  -o $(abspath lib/lib$(ARTIFACT)) \
	  -H:NativeLinkerOption=$(abspath target/native/shim.o) \
	  -H:NativeLinkerOption=-pthread \
	  -H:APIFunctionPrefix=snakeyaml_

test: test-java test-native test-goyaml test-parity

target/abi-test: test/abi.c include/yamlstar_plugin.h
	mkdir -p target
	$(CC) -std=c11 -Wall -Wextra -Werror -Iinclude test/abi.c \
	  $(if $(IS-MACOS),,-ldl) -pthread -o target/abi-test

test-native: $(LIB) target/abi-test
	target/abi-test $(abspath $(LIB))

test-goyaml: $(LIB) $(GO)
	CGO_ENABLED=0 $(GO) test ./goyaml

test-goyaml-race: $(LIB) $(GO)
	CGO_ENABLED=1 $(GO) test -race ./goyaml

test-goyaml-cli: $(LIB) $(GO) $(PERL) $(PYTHON)
	GO=$(GO) PERL=$(PERL) PYTHON=$(PYTHON) bash util/test-goyaml-cli

shellcheck: $(SHELLCHECK)
	$(SHELLCHECK) util/test-goyaml-cli

test-parity: test-java $(LIB) $(GO)
	SNAKEYAML_JAVA_CLASSPATH=$(abspath target/classes):$(ENGINE-JAR) \
	  $(GO) test ./goyaml -run TestJVMNativeParity

test-build-order: $(LIB) $(MAVEN)
	$(MVN) test
	$(MAKE) -W native/shim.c test-native

test-graal-host: $(LIB) $(GRAALVM)
	mkdir -p target/graal-host
	$(CC) -std=c11 -Wall -Wextra -Werror -DSNAKEYAML_GRAAL_HOST \
	  -Iinclude -c test/abi.c -o target/graal-host/probe.o
	$(GRAALVM-BIN)/javac \
	  -cp $(MAVEN-REPOSITORY)/org/graalvm/sdk/nativeimage/25.0.2/nativeimage-25.0.2.jar:$(MAVEN-REPOSITORY)/org/graalvm/sdk/word/25.0.2/word-25.0.2.jar \
	  -d target/graal-host test/GraalHost.java
	$(GRAALVM) --no-fallback -O1 -march=compatibility \
	  -cp target/graal-host -H:CCompilerOption=-I$(abspath test) \
	  -H:NativeLinkerOption=$(abspath target/graal-host/probe.o) \
	  $(if $(IS-MACOS),,-H:NativeLinkerOption=-ldl) \
	  -H:NativeLinkerOption=-pthread -o target/graal-host/probe GraalHost
	target/graal-host/probe $(abspath $(LIB))

check-versions: $(PERL)
	$(PERL) util/check-versions $(VERSION)

dist: check-versions test shellcheck
	rm -rf dist/$(RELEASE-NAME)
	mkdir -p dist/$(RELEASE-NAME)/lib dist/$(RELEASE-NAME)/include
	cp $(LIB) dist/$(RELEASE-NAME)/lib/
	cp include/yamlstar_plugin.h dist/$(RELEASE-NAME)/include/
	cp plugin.edn License ReadMe.md Releasing.md dist/$(RELEASE-NAME)/
	mkdir -p dist/$(RELEASE-NAME)/licenses
	cp licenses/* dist/$(RELEASE-NAME)/licenses/
	cp -RL $(GRAALVM-HOME)/legal dist/$(RELEASE-NAME)/licenses/graalvm
	tar -C dist -cJf $(ARCHIVE) $(RELEASE-NAME)
	cd dist && shasum -a 256 $(notdir $(ARCHIVE)) > SHA256SUMS
	$(MAKE) test-archive

test-archive: target/abi-test
	cd dist && shasum -a 256 -c SHA256SUMS
	rm -rf target/archive-test/$(RELEASE-NAME)
	mkdir -p target/archive-test
	tar -C target/archive-test -xf $(ARCHIVE)
	target/abi-test \
	  $(abspath target/archive-test/$(RELEASE-NAME)/lib/$(notdir $(LIB)))

bundle: $(MAVEN) $(PERL) $(YS)
	YS=$(YS) $(PERL) util/maven-release bundle \
	  $(MAVEN) $(MAVEN-REPOSITORY)

release-check: check-versions $(PERL)
	$(PERL) util/maven-release check-version '$(v)'
	@test -z "$$(git status --porcelain)" || { \
	  echo 'Commit reviewed release sources first' >&2; exit 1; }

release: release-check $(YS) $(PYTHON) $(GH)
	YS=$(YS) GH=$(GH) PERL=$(PERL) $(PYTHON) util/release '$(v)'

release-publish: release-check $(MAVEN) $(PYTHON) $(GH)
	GH=$(GH) PERL=$(PERL) MAVEN=$(MAVEN) \
	  MAVEN_REPOSITORY=$(MAVEN-REPOSITORY) \
	  $(PYTHON) util/release --publish '$(v)'

test-release: $(PERL) $(YS) $(PYTHON)
	YS=$(YS) $(PERL) test/maven-release.t
	$(PYTHON) -m unittest discover -s test -p '*_test.py'
