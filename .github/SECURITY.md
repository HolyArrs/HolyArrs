# Security

## Report a problem privately

Please do not open a public issue for a security problem. Report it in one of these ways:

- On GitHub: open the Security tab of https://github.com/HolyArrs/HolyArrs and click "Report a vulnerability".
- By email: support@HolyArrs.com

Tell us what you found, how to reproduce it, your HolyArrs version (Settings > This app, or the image tag) and how you run it (Docker, NAS, behind a reverse proxy).

We answer within 7 days and keep you posted while we fix it. When the fix ships, we credit you in the release notes unless you ask us not to.

## Check that a download is ours

Every image and Linux download is built and signed on GitHub, with a record of how it was built and a list of what is inside it. Check one with the [GitHub CLI](https://cli.github.com):

```bash
gh attestation verify oci://docker.io/holyarrs/holyarrs:<version> --repo HolyArrs/HolyArrs
gh attestation verify holyarrs_<version>_linux_amd64 --repo HolyArrs/HolyArrs
```

If it doesn't say the check passed, don't run that file. More in [Verify the image](https://github.com/HolyArrs/HolyArrs/blob/main/docs/run/updates.md#verify-the-image).

## Supported versions

Only the latest release gets security fixes. Please update before you report.
