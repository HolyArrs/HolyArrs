// Package files holds plain file-system helpers. It turns what the system
// says about a folder into plain words: which filesystem the folder is on
// (from /proc/self/mountinfo), what that means in everyday terms, why a write
// failed, which host user an ID belongs to and whether /config was mapped at
// all. It only reads text it is given and never decides whether HolyArrs
// starts. When it is unsure it returns "".
package files
