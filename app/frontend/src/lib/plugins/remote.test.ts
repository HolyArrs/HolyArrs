import { describe, expect, it } from 'vitest';
import { connectionLabel, remoteStateLabel, signOutNotice } from './view.ts';

describe('remote access wording', () => {
	it('names every Tailscale state in plain words', () => {
		expect(remoteStateLabel('not_connected')).toBe('Not connected yet');
		expect(remoteStateLabel('waiting_for_sign_in')).toBe('Waiting for sign-in');
		expect(remoteStateLabel('waiting_for_admin_approval')).toBe('Waiting for your tailnet admin to approve this device');
		expect(remoteStateLabel('connected')).toBe('Connected');
		expect(remoteStateLabel('login_expiring')).toBe('Connected · login expires within 14 days');
		expect(remoteStateLabel('login_expired')).toBe('Login expired. Sign in again.');
		expect(remoteStateLabel('device_deleted')).toBe('Removed in Tailscale. Sign in again.');
		expect(remoteStateLabel('something_new')).toBe('Not connected yet');
	});
	it('reads a stopped plugin as disconnected or starting', () => {
		expect(remoteStateLabel('connected', { running: false, enabled: false })).toBe('Disconnected (still signed in)');
		expect(remoteStateLabel('connected', { running: false, enabled: true })).toBe('Starting…');
		expect(remoteStateLabel('connected', { running: true, enabled: true })).toBe('Connected');
		expect(remoteStateLabel('connected', { running: false, enabled: false, disabled_reason: 'signed out by the owner' })).toBe('Not connected yet');
	});
	it('says direct or relayed', () => {
		expect(connectionLabel('direct')).toBe('Direct');
		expect(connectionLabel('relayed')).toBe('Relayed');
		expect(connectionLabel(undefined)).toBe('');
	});
	it('tells the owner to remove the device when Tailscale could not', () => {
		expect(signOutNotice({ removed_remotely: false })).toBe("Signed out here. Remove the device in Tailscale's admin page too.");
		expect(signOutNotice({ removed_remotely: true })).toBe('');
		expect(signOutNotice(undefined)).toBe('');
	});
});
