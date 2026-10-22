package app

import "errors"

// LaunchUpdate excludes device writes while starting the installer. A launch
// failure leaves the controller running. After success, stop discovery and
// reject new work before the desktop exits. Never cancel a device write to update.
func (c *Controller) LaunchUpdate(launch func(Settings) error) error {
	c.mu.Lock()
	if c.closed || c.configuring {
		c.mu.Unlock()
		return errors.New("finish the active device operation before installing an update")
	}
	if err := launch(c.settings); err != nil {
		c.mu.Unlock()
		return err
	}
	c.closed = true
	c.cancel()
	c.mu.Unlock()
	c.wg.Wait()
	return nil
}
