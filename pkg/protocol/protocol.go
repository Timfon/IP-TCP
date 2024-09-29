package protocol

import (
  "fmt"
)

//Capitalized function name means it is exported (public), lowercase means it is private
func PrintProtocol() {
  fmt.Println("shared code, protocol!")
}
