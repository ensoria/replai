// Package secret verifies that replai can import internal packages of the
// target project.
package secret

// Token returns a fixed value.
func Token() string {
	return "internal-token"
}
