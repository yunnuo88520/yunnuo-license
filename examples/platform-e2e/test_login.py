#!/usr/bin/env python3
"""Exercise ordinary login, optional image CAPTCHA and opt-in second-factor login."""
import argparse
import hashlib
import importlib.util
import json
import os
import pathlib
import secrets
import sqlite3
import subprocess
import tempfile
import time

from playwright.sync_api import sync_playwright

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('production', pathlib.Path(__file__).with_name('test_production.py'))
production = importlib.util.module_from_spec(spec)
spec.loader.exec_module(production)
http = production.http


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--server', required=True, type=pathlib.Path)
    parser.add_argument('--artifacts', required=True, type=pathlib.Path)
    parser.add_argument('--chromium', default='/usr/bin/chromium')
    parser.add_argument('--screenshots', action='store_true')
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {'result': 'running', 'checks': []}
    password = secrets.token_hex(24)
    with tempfile.TemporaryDirectory(prefix='login-flow-') as directory, (args.artifacts / 'server.log').open('w') as log:
        temporary = pathlib.Path(directory)
        database = temporary / 'db.sqlite'
        port = http.free_port()
        base = f'http://127.0.0.1:{port}'
        env = {k: v for k, v in os.environ.items() if not k.startswith('YN_')}
        env.update(YN_ADDR=f'127.0.0.1:{port}', YN_DB_DRIVER='sqlite', YN_DB=f'file:{database}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)', YN_DB_CONFIG_FILE=str(temporary / 'config.json'), YN_RELEASE_DIR=str(temporary / 'packages'))
        process = subprocess.Popen([str(args.server.resolve())], cwd=REPO / 'backend', env=env, stdout=log, stderr=log)
        try:
            http.wait_ready(base, '/readyz', process)
            http.initialize(base, 'admin', password, env['YN_DB'])
            root = http.api(base, '/admin/login', {'username': 'admin', 'password': password})['access_token']
            http.api(base, '/admin/users', {'username': 'two-step-admin', 'password': password, 'role': 'admin'}, root)
            operator = http.api(base, '/admin/login', {'username': 'two-step-admin', 'password': password})['access_token']
            setup = http.api(base, '/admin/mfa/begin', {'password': password}, operator)
            codes = http.api(base, '/admin/mfa/confirm', {'password': password, 'code': production.otp(setup['secret'])}, operator)['recovery_codes']
            with sync_playwright() as pw:
                browser = pw.chromium.launch(executable_path=None if args.chromium == 'bundled' else args.chromium, headless=True, args=['--no-sandbox'])
                errors = []

                def new_page():
                    context = browser.new_context(viewport={'width': 1440, 'height': 1000}, locale='zh-CN', timezone_id='Asia/Shanghai', color_scheme='light')
                    page = context.new_page()
                    page.on('pageerror', lambda error: errors.append(str(error)))
                    return context, page

                def visit(page):
                    with page.expect_response(lambda response: response.url.endswith('/v1/captcha')) as response:
                        page.goto(base + '/admin')
                    page.get_by_label('账号', exact=True).wait_for()
                    return response.value.json()['data']

                def snapshot(page, name, mobile=False, dark=False):
                    page.set_viewport_size({'width': 390 if mobile else 1440, 'height': 844 if mobile else 1000})
                    page.emulate_media(color_scheme='dark' if dark else 'light')
                    page.locator("html[data-theme='" + ('dark' if dark else 'light') + "']").wait_for(state='attached')
                    assert page.evaluate('document.documentElement.scrollWidth <= innerWidth + 1')
                    if args.screenshots:
                        page.screenshot(path=str(args.artifacts / (name + '.png')), full_page=True, animations='disabled')

                context, page = new_page()
                assert not visit(page)['enabled']
                assert page.locator('.login-form input').count() == 2
                assert '恢复码' not in page.locator('.login-box').inner_text()
                assert page.get_by_label('验证器验证码', exact=True).count() == 0
                snapshot(page, 'login-default-desktop')
                snapshot(page, 'login-default-mobile', mobile=True)
                page.get_by_label('账号', exact=True).fill('admin')
                page.get_by_label('密码', exact=True).fill(password)
                page.get_by_role('button', name='安全登录', exact=True).click()
                page.locator('.console').wait_for()
                report['checks'].append('An ordinary account sees only username and password and logs in directly; the default desktop/mobile page never asks for a recovery code')
                context.close()

                http.api(base, '/admin/captcha-settings', {'login_enabled': True}, root)
                context, page = new_page()
                captcha = visit(page)
                assert captcha['enabled'] and captcha['image'].startswith('data:image/png;base64,')
                page.get_by_alt_text('图形验证码', exact=True).wait_for()
                assert page.get_by_label('验证器验证码', exact=True).count() == 0
                snapshot(page, 'login-image-captcha')
                with page.expect_response(lambda response: response.url.endswith('/v1/captcha')) as refreshed:
                    page.get_by_role('button', name='刷新验证码', exact=True).click()
                next_captcha = refreshed.value.json()['data']
                assert next_captcha['id'] != captcha['id']
                # Seed a known answer only in this isolated test database. The
                # production endpoint still generates a random image and code.
                def known_answer(challenge):
                    with sqlite3.connect(database) as db:
                        changed = db.execute('UPDATE captcha_challenges SET code_hash=? WHERE id=?', (hashlib.sha256(b'ABCD').hexdigest(), challenge['id']))
                        assert changed.rowcount == 1
                known_answer(next_captcha)
                page.get_by_label('账号', exact=True).fill('two-step-admin')
                page.get_by_label('密码', exact=True).fill(password)
                page.get_by_label('图形验证码', exact=True).fill('ZZZZ')
                with page.expect_response(lambda response: response.url.endswith('/v1/captcha')) as refreshed:
                    with page.expect_response(lambda response: response.url.endswith('/admin/login/start')) as denied:
                        page.get_by_role('button', name='安全登录', exact=True).click()
                    assert denied.value.status == 401 and denied.value.json()['error']['code'] == 'CAPTCHA_INVALID'
                known_answer(refreshed.value.json()['data'])
                page.get_by_label('图形验证码', exact=True).fill('ABCD')
                with page.expect_response(lambda response: response.url.endswith('/admin/login/start')) as started:
                    page.get_by_role('button', name='安全登录', exact=True).click()
                proof = started.value.json()['data']
                assert 'login' not in proof and proof['mfa']['challenge']
                page.get_by_role('heading', name='两步验证', exact=True).wait_for()
                assert not page.evaluate("sessionStorage.getItem('yn.admin_token')")
                assert page.get_by_label('密码', exact=True).count() == 0
                assert page.get_by_label('图形验证码', exact=True).count() == 0
                assert page.get_by_label('备用恢复码', exact=True).count() == 0
                snapshot(page, 'login-two-step')
                report['checks'].append('The administrator-controlled image CAPTCHA appears with refresh, rejects a wrong answer and is consumed once; only a correct password plus CAPTCHA reaches the second step')

                valid = {production.otp(setup['secret'], time.time() + delta) for delta in [-30, 0, 30]}
                wrong = next(f'{n:06d}' for n in range(10) if f'{n:06d}' not in valid)
                page.get_by_label('验证器验证码', exact=True).fill(wrong)
                with page.expect_response(lambda response: response.url.endswith('/admin/login/mfa')) as denied:
                    page.get_by_role('button', name='验证并登录', exact=True).click()
                assert denied.value.status == 403 and denied.value.json()['error']['code'] == 'MFA_CODE_INVALID'
                assert set(denied.value.request.post_data_json) == {'challenge', 'code'}
                page.get_by_role('alert').filter(has_text='验证码无效').wait_for()
                assert page.get_by_label('图形验证码', exact=True).count() == 0
                page.get_by_role('button', name='无法使用验证器？使用备用恢复码', exact=True).click()
                page.get_by_label('备用恢复码', exact=True).wait_for()
                snapshot(page, 'login-recovery-mobile-dark', mobile=True, dark=True)
                page.get_by_label('备用恢复码', exact=True).fill(codes[0])
                with page.expect_response(lambda response: response.url.endswith('/admin/login/mfa')) as completed:
                    page.get_by_role('button', name='验证并登录', exact=True).click()
                assert completed.value.status == 200
                assert set(completed.value.request.post_data_json) == {'challenge', 'code'}
                page.locator('.console').wait_for()
                assert http.http(base, '/admin/login/mfa', {'challenge': proof['mfa']['challenge'], 'code': codes[1]}, expected=401)['error']['code'] == 'MFA_CHALLENGE_INVALID'
                report['checks'].append('An incorrect verifier code remains in the second step without asking for CAPTCHA/password again; recovery is shown only by explicit choice and a completed challenge cannot be reused')
                context.close()

                http.api(base, '/admin/captcha-settings', {'login_enabled': False}, root)
                context, page = new_page()
                assert not visit(page)['enabled']
                page.get_by_label('账号', exact=True).fill('two-step-admin')
                page.get_by_label('密码', exact=True).fill(password)
                page.get_by_role('button', name='安全登录', exact=True).click()
                page.get_by_role('heading', name='两步验证', exact=True).wait_for()
                page.get_by_role('button', name='返回账号密码登录', exact=True).click()
                page.get_by_label('密码', exact=True).wait_for()
                assert page.get_by_label('密码', exact=True).input_value() == ''
                assert page.get_by_label('备用恢复码', exact=True).count() == 0
                page.get_by_label('密码', exact=True).fill(password)
                page.get_by_role('button', name='安全登录', exact=True).click()
                page.get_by_label('验证器验证码', exact=True).wait_for()
                # Use the accepted next clock window to avoid reusing the TOTP
                # already consumed when enabling MFA a few seconds earlier.
                page.get_by_label('验证器验证码', exact=True).fill(production.otp(setup['secret'], time.time() + 30))
                page.get_by_role('button', name='验证并登录', exact=True).click()
                page.locator('.console').wait_for()
                report['checks'].append('CAPTCHA and optional two-factor settings remain independent; returning to password login clears the password; a verifier TOTP completes the second step')
                assert not errors, errors
                context.close()
                browser.close()
                report['result'] = 'passed'
        finally:
            process.terminate()
            process.wait(timeout=20)
            (args.artifacts / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2))
    print(json.dumps({'result': report['result'], 'checks': len(report['checks'])}))


if __name__ == '__main__':
    main()
