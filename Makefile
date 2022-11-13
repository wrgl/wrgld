SHELL = /bin/bash
.DEFAULT_GOAL = all

GO := go
MD5 := md5
DOCKER := docker
BUILD_DIR := build
MD5_DIR := $(BUILD_DIR)/md5
OS_ARCHS := linux-amd64 darwin-amd64 darwin-arm64
BINARIES := $(foreach osarch,$(OS_ARCHS),$(BUILD_DIR)/wrgld-$(osarch)/bin/wrgld)
WRGLD_TAR_FILES := $(foreach osarch,$(OS_ARCHS),$(BUILD_DIR)/wrgld-$(osarch).tar.gz)
IMAGES := $(BUILD_DIR)/wrgld.image
WRGLD_STATIC_ASSETS := $(wildcard wrgld/pkg/auth/oauth2/static/*) $(wildcard wrgld/pkg/auth/oauth2/templates/*)
COMMON_LDFLAGS = -X github.com/wrgl/wrgld/cmd.version=$(VERSION)

.PHONY: all clean images
all: $(WRGLD_TAR_FILES)
images: $(IMAGES)
clean:
	rm -rf $(BUILD_DIR)

# Check that given variables are set and all have non-empty values,
# die with an error otherwise.
#
# Params:
#   1. Variable name(s) to test.
#   2. (optional) Error message to print.
check_defined = \
    $(strip $(foreach 1,$1, \
        $(call __check_defined,$1,$(strip $(value 2)))))
__check_defined = \
    $(if $(value $1),, \
      $(error Undefined $1$(if $2, ($2))))

$(call check_defined, VERSION)

define binary_rule =
echo "\$$(BUILD_DIR)/wrgld-$(2)-$(1)/bin/wrgld: \$$(MD5_DIR)/go.sum.md5 \$$(wrgld_SOURCES)" >> $(3) && \
echo -e "\t@-mkdir -p \$$(dir \$$@) 2>/dev/null" >> $(3) && \
(if [ "$(2)" == "linux" ]; then \
  echo -e "\tenv CC=x86_64-linux-musl-gcc CXX=x86_64-linux-musl-g++ GOARCH=amd64 GOOS=linux CGO_ENABLED=1 go build -ldflags \"-linkmode external -extldflags -static \$$(COMMON_LDFLAGS)\" -a -o \$$@ github.com/wrgl/wrgld" >> $(3); \
else \
  echo -e "\tCGO_ENABLED=1 GOARCH=$(1) GOOS=$(2) go build -ldflags \"\$$(COMMON_LDFLAGS)\" -a -o \$$@ github.com/wrgl/wrgld" >> $(3); \
fi) && \
echo "" >> $(3)

endef

$(BUILD_DIR)/wrgld.d: | $(BUILD_DIR)
	echo "wrgld_SOURCES =" > $@
	echo "$$($(GO) list -deps github.com/wrgl/wrgld | \
		grep github.com/wrgl/wrgld/ | \
		sed -r -e 's/github.com\/wrgl\/wrgld\/(.+)/\1/g' | \
		xargs -n 1 -I {} find {} -maxdepth 1 -name '*.go' \! -name '*_test.go' -print | \
		sed -r -e 's/(.+)/$(subst /,\/,wrgld_SOURCES += $(MD5_DIR))\/\1.md5/g')" >> $@
	echo "" >> $@
	$(foreach osarch,$(OS_ARCHS),$(call binary_rule,$(word 2,$(subst -, ,$(osarch))),$(word 1,$(subst -, ,$(osarch))),$@))

define wrgld_bin_rule =
$(BUILD_DIR)/wrgld-$(1)/bin/wrgld: $$(patsubst %,$$(MD5_DIR)/%.md5,$$(WRGLD_STATIC_ASSETS))
endef

define license_rule =
$(BUILD_DIR)/wrgld-$(1)/LICENSE: LICENSE
	cp $$< $$@
endef

define tar_rule =
$(BUILD_DIR)/wrgld-$(1).tar.gz: $(BUILD_DIR)/wrgld-$(1)/bin/wrgld $(BUILD_DIR)/wrgld-$(1)/LICENSE
	cd $(BUILD_DIR) && \
	tar -czvf $$(notdir $$@) wrgld-$(1)
endef

# calculate md5
$(MD5_DIR)/%.md5: % | $(MD5_DIR)
	@-mkdir -p $(dir $@) 2>/dev/null
	$(if $(filter-out $(shell cat $@ 2>/dev/null),$(shell $(MD5) $<)),$(MD5) $< > $@)

$(foreach osarch,$(OS_ARCHS),$(eval $(call wrgld_bin_rule,$(osarch))))

$(foreach osarch,$(OS_ARCHS),$(eval $(call license_rule,$(osarch))))

$(foreach osarch,$(OS_ARCHS),$(eval $(call tar_rule,$(osarch))))

$(BUILD_DIR)/wrgld.image: Dockerfile $(BUILD_DIR)/wrgld-linux-amd64/bin/wrgld $(BUILD_DIR)/wrgld-linux-amd64/LICENSE
	$(DOCKER) build -t wrgld:latest -f Dockerfile $(BUILD_DIR)/wrgld-linux-amd64
	$(DOCKER) tag wrgld:latest wrgld:$(VERSION)
	$(DOCKER) images --format '{{.ID}}' wrgld:latest > $@

$(BUILD_DIR): ; @-mkdir $@ 2>/dev/null
$(MD5_DIR): | $(BUILD_DIR) ; @-mkdir $@ 2>/dev/null

include $(BUILD_DIR)/wrgld.d