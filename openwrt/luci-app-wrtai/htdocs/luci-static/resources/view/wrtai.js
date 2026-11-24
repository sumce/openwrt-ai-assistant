'use strict';
'require view';
'require form';

/*
 * luci-app-wrtai — deliberately minimal: only the enable switch and the
 * listen port. Everything else (model providers, security, backups) lives
 * in the WRT AI web UI at http://<router-ip>:<port>.
 */
return view.extend({
	render: function () {
		var m, s, o;

		m = new form.Map('wrtai', _('WRT AI'),
			_('AI 助手。启用后访问 http://<路由器IP>:<端口> 进入 WRT AI 界面。') +
			'<br/>' +
			_('模型配置、安全策略与备份都在 WRT AI 界面内管理，此处只控制服务的开关与端口。'));

		s = m.section(form.NamedSection, 'main', 'wrtai', _('服务设置'));
		s.addremove = false;

		o = s.option(form.Flag, 'enabled', _('启用'));
		o.rmempty = false;

		o = s.option(form.Value, 'port', _('监听端口'));
		o.datatype = 'port';
		o.default = '8765';
		o.rmempty = false;
		o.placeholder = '8765';

		return m.render();
	},
});
