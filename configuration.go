// Copyright 2017 Axel Etcheverry. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package main

import (
	"github.com/rs/xlog"
	"github.com/spf13/viper"
)

var (
	basePath = "/var/lib/" + AppName
)

func init() {
	viper.SetDefault("logger.level", "info")
	viper.SetDefault("server.host", "")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("image.source.provider", "fs")
	viper.SetDefault("image.source.fs.path", basePath+"/source")
	viper.SetDefault("image.cache.provider", "fs")
	viper.SetDefault("image.cache.fs.path", basePath+"/cache")
	viper.SetDefault("image.support.extensions", map[string]bool{
		"jpg":  true,
		"jpeg": true,
		"webp": true,
		"png":  true,
		"tiff": true,
	})

	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/" + AppName + "/")   // path to look for the config file in
	viper.AddConfigPath("$HOME/." + AppName + "/") // call multiple times to add many search paths
	viper.AddConfigPath(".")

	// Find and read the config file
	if err := viper.ReadInConfig(); err != nil {
		xlog.Info(err)
	}
	viper.SetEnvPrefix("IS")
	viper.AutomaticEnv()
}

type SourceFSConfiguration struct {
	Path string
}

type ImageSourceConfiguration struct {
	Provider string
	FS       *SourceFSConfiguration
}

type CacheFSConfiguration struct {
	Path string
}

type ImageCacheConfiguration struct {
	Provider string
	FS       *CacheFSConfiguration
}

type ImageSupportConfiguration struct {
	Extensions map[string]interface{}
}

func (c ImageSupportConfiguration) IsExtSupported(ext string) bool {
	enable, ok := c.Extensions[ext]

	return (ok && enable.(bool))
}

type ImageConfiguration struct {
	Source  *ImageSourceConfiguration
	Cache   *ImageCacheConfiguration
	Support *ImageSupportConfiguration
}

type ServerConfiguration struct {
	Host string
	Port int
}

type LoggerConfiguration struct {
	Level string
}

type Configuration struct {
	Logger *LoggerConfiguration
	Server *ServerConfiguration
	Image  *ImageConfiguration
}

// NewConfiguration constructor
func NewConfiguration() *Configuration {
	return &Configuration{
		Logger: &LoggerConfiguration{
			Level: viper.GetString("logger.level"),
		},
		Server: &ServerConfiguration{
			Host: viper.GetString("server.host"),
			Port: viper.GetInt("server.port"),
		},
		Image: &ImageConfiguration{
			Source: &ImageSourceConfiguration{
				Provider: viper.GetString("image.source.provider"),
				FS: &SourceFSConfiguration{
					Path: viper.GetString("image.source.fs.path"),
				},
			},
			Cache: &ImageCacheConfiguration{
				Provider: viper.GetString("image.cache.provider"),
				FS: &CacheFSConfiguration{
					Path: viper.GetString("image.cache.fs.path"),
				},
			},
			Support: &ImageSupportConfiguration{
				Extensions: viper.GetStringMap("image.support.extensions"),
			},
		},
	}
}
