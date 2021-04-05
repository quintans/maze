package maze

import "github.com/quintans/toolkit/web"

// ResponseBuffer buffers the response, permitting setting headers after starting writing the response.
func ResponseBuffer(c IContext) error {
	rec := web.NewBufferedResponse()
	w := c.GetResponse()
	// passing a buffer instead of the original RW
	c.SetResponse(rec)
	// restores the original response, even in the case of a panic
	defer func() {
		c.SetResponse(w)
	}()
	err := c.Proceed()
	if err == nil {
		rec.Flush(w)
	}

	return err
}

func StaticGz(dir string) func(c IContext) error {
	rs := web.ResourcesHandler(dir)
	return func(c IContext) error {
		rs.ServeHTTP(c.GetResponse(), c.GetRequest())
		return nil
	}
}
