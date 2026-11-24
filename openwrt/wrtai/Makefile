#
# Copyright (c) 2026 WRT AI Contributors
# MIT licensed, see the repository LICENSE.
#
# Buildroot package wrapping the prebuilt Go binary. Build the binaries
# first from the repository root (`make cross`), then point this package at
# the matching dist/ directory.

include $(TOPDIR)/rules.mk

PKG_NAME:=wrtai
PKG_VERSION:=0.1.1
PKG_RELEASE:=1
PKG_MAINTAINER:=WRT AI Contributors
PKG_LICENSE:=MIT

PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk

define Package/wrtai
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=WRT AI - AI assistant for OpenWRT
  URL:=https://github.com/sumce/openwrt-ai-assistant
endef

define Package/wrtai/description
  AI assistant that runs on your router: chat in a browser and let the AI
  read status/logs, run diagnostics and modify UCI configuration through a
  graded approval + backup/rollback safety system.
endef

define Build/Prepare
	$(INSTALL_DIR) $(PKG_BUILD_DIR)
	# Copy the prebuilt binary for this target arch from the repo dist/.
	# $$(ARCH) is the buildroot arch (e.g. mipsel_24kc, aarch64_cortex-a53);
	# install.sh / the repo Makefile produce dist/<goarch>/wrtai.
	case "$(ARCH)" in \
	  *aarch64*) GOARCH=arm64 ;; \
	  *mips64*) GOARCH=mips64 ;; \
	  mips|mips_*) GOARCH=mips ;; \
	  *mipsel*|*mips**) GOARCH=mipsle ;; \
	  *arm*) GOARCH=arm ;; \
	  *x86_64*|*amd64*) GOARCH=amd64 ;; \
	  *) GOARCH=$(ARCH) ;; \
	esac; \
	$(CP) $(CURDIR)/../../dist/linux-$$(GOARCH)/wrtai $(PKG_BUILD_DIR)/wrtai || \
	$(CP) $(CURDIR)/dist/linux-$$(GOARCH)/wrtai $(PKG_BUILD_DIR)/wrtai
	chmod +x $(PKG_BUILD_DIR)/wrtai
endef

define Build/Compile
	true
endef

define Package/wrtai/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/wrtai $(1)/usr/bin/wrtai
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/wrtai.init $(1)/etc/init.d/wrtai
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/wrtai.uci $(1)/etc/config/wrtai
	$(INSTALL_DIR) $(1)/etc/wrtai
endef

$(eval $(call BuildPackage,wrtai))
