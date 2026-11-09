// Package secretsfile reads the owner's secrets file, one YAML file with the
// keys, passwords and addresses for outside services, and saves each filled
// item through the same code the Settings screens use, so every secret ends
// up in the vault. Results and errors only name the item, a key or a line
// number and never hold a value from the file, so nothing secret can leak
// into logs or the screen.
package secretsfile
