// Code generated by protoc-gen-resource-types. DO NOT EDIT.

package demov2

import (
	"github.com/hashicorp/consul/proto-public/pbresource"
)

const (
	GroupName = "demo"
	Version   = "v2"

	AlbumKind    = "Album"
	ArtistKind   = "Artist"
	FestivalKind = "Festival"
)

var (
	AlbumType = &pbresource.Type{
		Group:        GroupName,
		GroupVersion: Version,
		Kind:         AlbumKind,
	}

	ArtistType = &pbresource.Type{
		Group:        GroupName,
		GroupVersion: Version,
		Kind:         ArtistKind,
	}

	FestivalType = &pbresource.Type{
		Group:        GroupName,
		GroupVersion: Version,
		Kind:         FestivalKind,
	}
)
