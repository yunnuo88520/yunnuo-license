#!/usr/bin/env python3
"""Password confirmation, retained sessions on errors, and revocation after success."""
import argparse
import importlib.util
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import uuid

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('demo_http', REPO / 'examples/update-demo/test_e2e.py')
http = importlib.util.module_from_spec(spec)
spec.loader.exec_module(http)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--server', type=pathlib.Path)
    parser.add_argument('--base-url')
    parser.add_argument('--artifacts', required=True, type=pathlib.Path)
    parser.add_argument('--screenshots', action='store_true')
    parser.add_argument('--chromium', default='/usr/bin/chromium')
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    password = os.environ['YN_TEST_ADMIN_PASSWORD'] if args.base_url else secrets.token_hex(24)
    report = {'result': 'running', 'database': 'mysql' if args.base_url else 'sqlite', 'checks': []}
    process = browser = pw = None
    with tempfile.TemporaryDirectory(prefix='password-e2e-') as temporary, (args.artifacts / 'server.log').open('w') as log:
        try:
            base = args.base_url
            if not base:
                port = http.free_port()
                base = f'http://127.0.0.1:{port}'
                env = {k: v for k, v in os.environ.items() if not k.startswith('YN_')}
                env.update(YN_ADDR=f'127.0.0.1:{port}', YN_DB_DRIVER='sqlite', YN_DB=f'file:{temporary}/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)', YN_DB_CONFIG_FILE=temporary+'/config.json', YN_RELEASE_DIR=temporary+'/packages', YN_TRUST_PROXY_HEADERS='true')
                process = subprocess.Popen([str(args.server.resolve())], cwd=REPO / 'backend', env=env, stdout=log, stderr=log)
                http.wait_ready(base, '/readyz', process)
                http.initialize(base, 'admin', password, env['YN_DB'])

            def call(path, data=None, token='', expected=200, ip='198.51.100.200'):
                result = http.http(base, path, data, token, expected=expected, extra_headers={'X-Forwarded-For': ip})
                return result['data'] if expected == 200 else result['error']['code']

            root = call('/admin/login', {'username': 'admin', 'password': password})['access_token']
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                pw = sync_playwright().start()
                browser = pw.chromium.launch(executable_path=None if args.chromium == 'bundled' else args.chromium, headless=True, args=['--no-sandbox'])
            errors = []
            for index, portal in enumerate(['admin', 'developer', 'user', 'agent']):
                username = 'password-' + portal + '-' + uuid.uuid4().hex[:6]
                original = secrets.token_hex(20)
                api_password = ' api-' + secrets.token_hex(16) + ' '
                ip = '198.51.100.' + str(210 + index)
                if portal in ['admin', 'developer']:
                    call('/admin/users', {'username': username, 'password': original, 'role': portal}, root)
                elif portal == 'user':
                    call('/admin/customers', {'username': username, 'password': original}, root)
                else:
                    call('/admin/agent-accounts', {'username': username, 'password': original, 'name': '密码验收代理'}, root)
                prefix = '/' + portal
                change_path = '/user/password' if portal == 'agent' else prefix + '/password'
                token = call(prefix+'/login', {'username': username, 'password': original}, ip=ip)['access_token']
                def body(current, new, confirmation):
                    return {'current_password': current, 'new_password': new, 'confirm_password': confirmation}
                assert call(change_path, body('incorrect-current', api_password, api_password), token, 400, ip) == 'CURRENT_PASSWORD_INCORRECT'
                assert call(change_path, body(original, api_password, 'mismatch-password'), token, 400, ip) == 'PASSWORD_CONFIRMATION_MISMATCH'
                assert call(prefix+'/profile', token=token, ip=ip)['username'] == username
                assert call(change_path, body(original, api_password, api_password), token, ip=ip)['changed']
                assert call(prefix+'/profile', token=token, expected=401, ip=ip) in ['INVALID_ADMIN_TOKEN', 'INVALID_USER_TOKEN', 'INVALID_AGENT_TOKEN']
                assert call(prefix+'/login', {'username': username, 'password': original}, expected=401, ip=ip) == 'INVALID_CREDENTIALS'
                token = call(prefix+'/login', {'username': username, 'password': api_password}, ip=ip)['access_token']
                if portal == 'agent':
                    assert call('/user/profile', token=token, ip=ip)['username'] == username
                report['checks'].append(portal+': wrong current password/confirmation preserve the session; success revokes it, rejects the old password and accepts the exact new password including spaces')
                if not browser:
                    continue

                context = browser.new_context(viewport={'width': 1440, 'height': 1050}, locale='zh-CN', timezone_id='Asia/Shanghai', extra_http_headers={'X-Forwarded-For': ip})
                page = context.new_page()
                page.on('pageerror', lambda error: errors.append(str(error)))
                route = {'admin': '/admin', 'developer': '/developer-console/', 'user': '/user-console/', 'agent': '/agent-console/'}[portal]
                key = {'admin': 'yn.admin_token', 'developer': 'yn.developer_token', 'user': 'yn.account_token', 'agent': 'yn.account_token'}[portal]
                login_button = {'admin': '安全登录', 'developer': '登录', 'user': '登录用户中心', 'agent': '进入工作台'}[portal]
                login_label = '登录名' if portal == 'agent' else '账号'
                page.goto(base+route)
                page.get_by_label(login_label, exact=True).fill(username)
                page.get_by_label('密码', exact=True).fill(api_password)
                page.get_by_role('button', name=login_button, exact=True).click()
                if portal in ['admin', 'agent']:
                    page.locator('.console').wait_for()
                else:
                    page.get_by_text('我的产品' if portal == 'developer' else '激活新授权', exact=True).wait_for()
                page.wait_for_load_state('networkidle')
                saved = page.evaluate('(key) => sessionStorage.getItem(key)', key)
                assert saved
                if portal in ['admin', 'agent']:
                    page.get_by_role('button', name=username, exact=False).click()
                    page.get_by_role('menuitem', name='修改密码', exact=True).click()
                    form = page.get_by_role('dialog', name='修改密码', exact=True).locator('.password-change-form')
                else:
                    page.get_by_text('修改密码' if portal == 'developer' else '修改登录密码', exact=True).click()
                    form = page.locator('.password-change-form')
                form.wait_for()
                assert form.locator('input[type=password]').count() == 3
                posted = []
                page.on('request', lambda req: posted.append(req.url) if req.url.endswith(change_path) and req.method == 'POST' else None)
                new_password = ' browser-' + secrets.token_hex(16) + ' '
                form.get_by_label('当前密码', exact=True).fill(api_password)
                form.get_by_label('新密码', exact=True).fill(new_password)
                form.get_by_label('确认新密码', exact=True).fill('mismatch-password')
                form.get_by_role('button').click()
                form.get_by_role('alert').filter(has_text='两次输入的新密码不一致').wait_for()
                assert not posted, 'mismatch was submitted to the server'
                form.get_by_label('当前密码', exact=True).fill('incorrect-current')
                form.get_by_label('确认新密码', exact=True).fill(new_password)
                with page.expect_response(lambda response: response.url.endswith(change_path) and response.request.method == 'POST') as response:
                    form.get_by_role('button').click()
                assert response.value.status == 400 and response.value.json()['error']['code'] == 'CURRENT_PASSWORD_INCORRECT'
                form.get_by_role('alert').filter(has_text='当前密码不正确').wait_for()
                assert page.evaluate('(key) => sessionStorage.getItem(key)', key) == saved
                assert call(prefix+'/profile', token=saved, ip=ip)['username'] == username
                capture = page.get_by_role('dialog', name='修改密码', exact=True) if portal in ['admin', 'agent'] else form
                capture.screenshot(path=str(args.artifacts / (portal+'-incorrect-current-password.png')), animations='disabled')
                if portal == 'admin':
                    page.set_viewport_size({'width': 390, 'height': 844})
                    assert page.evaluate('document.documentElement.scrollWidth <= innerWidth + 1')
                    capture.screenshot(path=str(args.artifacts / 'admin-password-mobile.png'), animations='disabled')
                    page.set_viewport_size({'width': 1440, 'height': 1050})

                # A form error from an older API can still be 401. It must not
                # be treated as a revoked session merely because of its status.
                page.route('**'+change_path, lambda interception: interception.fulfill(status=401, content_type='application/json', body=json.dumps({'success': False, 'error': {'code': 'INVALID_CREDENTIALS', 'message': 'incorrect current password'}})))
                form.get_by_label('当前密码', exact=True).fill(api_password)
                form.get_by_role('button').click()
                form.get_by_role('alert').filter(has_text='账号或密码不正确').wait_for()
                assert page.evaluate('(key) => sessionStorage.getItem(key)', key) == saved
                page.unroute('**'+change_path)
                with page.expect_response(lambda response: response.url.endswith(change_path) and response.request.method == 'POST') as response:
                    form.get_by_role('button').click()
                assert response.value.status == 200
                page.get_by_role('status').filter(has_text='密码已修改').wait_for()
                page.get_by_label(login_label, exact=True).wait_for()
                assert not page.evaluate('(key) => sessionStorage.getItem(key)', key)
                assert call(prefix+'/profile', token=saved, expected=401, ip=ip) in ['INVALID_ADMIN_TOKEN', 'INVALID_USER_TOKEN', 'INVALID_AGENT_TOKEN']
                page.get_by_label(login_label, exact=True).fill(username)
                page.get_by_label('密码', exact=True).fill(new_password)
                page.get_by_role('button', name=login_button, exact=True).click()
                if portal in ['admin', 'agent']:
                    page.locator('.console').wait_for()
                else:
                    page.get_by_text('我的产品' if portal == 'developer' else '激活新授权', exact=True).wait_for()
                report['checks'].append(portal+': three password fields, client mismatch rejection, inline current-password error with retained session, legacy 401 form-error handling and successful re-login are verified in Chromium')
                context.close()
            assert not errors, errors
            report['result'] = 'passed'
        finally:
            if browser:
                browser.close()
            if pw:
                pw.stop()
            if process:
                process.terminate()
                process.wait(timeout=20)
            (args.artifacts / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2))
    print(json.dumps({'result': report['result'], 'checks': len(report['checks']), 'database': report['database']}))


if __name__ == '__main__':
    main()
