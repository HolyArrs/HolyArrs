# Publishing a plugin

Plugins are not kept in this repo. Their source, catalog and publish workflows
live in [HolyArrs/HolyArrs-Plugins](https://github.com/HolyArrs/HolyArrs-Plugins):

- Our own plugins: `plugins/<id>/` there (Tailscale is `plugins/tailscale`).
- Community plugins: their authors' own repos, listed in `catalog/active/`.
- Publish and block: its `publish.yml` and `block.yml` workflows.

The steps are in [Publish plugins](dev/releasing.md#publish-plugins), what the
workflow does is in [Plugins](dev/plugins.md), and the keys are in
[the plugin signing key](plugin-signing-key.md).
