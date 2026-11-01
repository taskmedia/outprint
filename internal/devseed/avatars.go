package devseed

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

const avatarSize = 256

// Outline colours an avatar without an image from the id of its user or
// workspace, which is random for every fresh database: the documentation
// screenshots would change with each rebuilt stack. Avatars with an image
// look the same every time.
var (
	workspaceAvatarColour = color.RGBA{R: 0x1f, G: 0x6f, B: 0x8b, A: 0xff}
	userAvatarColour      = color.RGBA{R: 0x8e, G: 0x59, B: 0xff, A: 0xff}
)

func solidPNG(c color.Color) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, avatarSize, avatarSize))
	for y := 0; y < avatarSize; y++ {
		for x := 0; x < avatarSize; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ensureAvatars gives the workspace and the signed-in user a fixed avatar. It
// leaves one alone that already has an image, so a rerun uploads nothing.
func (s *seeder) ensureAvatars(ctx context.Context) error {
	var auth struct {
		User struct {
			ID        string `json:"id"`
			AvatarURL string `json:"avatarUrl"`
		} `json:"user"`
		Team struct {
			AvatarURL string `json:"avatarUrl"`
		} `json:"team"`
	}
	if err := s.c.Call(ctx, "auth.info", map[string]any{}, &auth); err != nil {
		return err
	}
	if err := s.setAvatar(ctx, "workspace", "workspace-avatar.png", workspaceAvatarColour, auth.Team.AvatarURL, "teams.update", nil); err != nil {
		return err
	}
	return s.setAvatar(ctx, "user", "user-avatar.png", userAvatarColour, auth.User.AvatarURL, "users.update", map[string]any{"id": auth.User.ID})
}

func (s *seeder) setAvatar(ctx context.Context, what, name string, colour color.Color, current, method string, params map[string]any) error {
	if current != "" {
		return nil
	}
	data, err := solidPNG(colour)
	if err != nil {
		return err
	}
	att, err := s.c.UploadAvatar(ctx, name, data)
	if err != nil {
		return fmt.Errorf("upload the %s avatar: %w", what, err)
	}
	s.logf("setting the %s avatar", what)
	body := map[string]any{"avatarUrl": att.URL}
	for k, v := range params {
		body[k] = v
	}
	return s.c.Call(ctx, method, body, nil)
}
