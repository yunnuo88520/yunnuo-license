"""Desktop example for the same generic client.py distribution.
python gui.py — requires Tk, plus requirements.txt; card is never persisted.
"""
import json, os, pathlib, subprocess, sys, threading, tkinter as tk
from tkinter import ttk
APP_ID="@@APP_ID@@"
class Window:
 def __init__(self,root):
  self.root=root;root.title("应用授权 · "+APP_ID);root.geometry("720x620")
  self.base=tk.StringVar(value=os.environ.get("YN_LICENSE_URL","@@LICENSE_URL@@"));self.kind=tk.StringVar(value=os.environ.get("YN_BINDING_TYPE","@@DEFAULT_BINDING@@"));self.value=tk.StringVar(value=os.environ.get("YN_BINDING_VALUE",""));self.card=tk.StringVar();self.invite_id=tk.StringVar();self.invite_token=tk.StringVar();self.pending=tk.StringVar();self.status=tk.StringVar(value="未激活 · 输入购买的卡密，或填写安装邀请")
  style=ttk.Style();style.theme_use("clam");style.configure("TFrame",background="#f4f7f5");style.configure("TLabel",background="#f4f7f5",foreground="#1d2b25",font=("sans",11));style.configure("TButton",padding=9);style.configure("Title.TLabel",font=("sans",22,"bold"))
  frame=ttk.Frame(root,padding=28);frame.pack(fill="both",expand=True);frame.columnconfigure(1,weight=1)
  ttk.Label(frame,text="产品授权",style="Title.TLabel").grid(row=0,column=0,columnspan=2,sticky="w");ttk.Label(frame,text="公开 AppID："+APP_ID).grid(row=1,column=0,columnspan=2,sticky="w",pady=(6,20))
  fields=[("授权站",self.base),("绑定值（安装标识可留空）",self.value),("激活卡密",self.card),("安装邀请 ID（可选）",self.invite_id),("安装邀请令牌（可选）",self.invite_token),("已批准的申请 ID（可选）",self.pending)]
  ttk.Label(frame,text="绑定类型").grid(row=2,column=0,sticky="w",pady=6);ttk.Combobox(frame,textvariable=self.kind,values=("installation","machine_id","domain","ip","seat","account","tenant","cluster"),state="readonly").grid(row=2,column=1,sticky="ew",pady=6)
  for row,(label,var) in enumerate(fields,3):ttk.Label(frame,text=label).grid(row=row,column=0,sticky="w",padx=(0,14),pady=7);ttk.Entry(frame,textvariable=var,show="•" if var in (self.card,self.invite_token) else "").grid(row=row,column=1,sticky="ew",pady=7)
  actions=ttk.Frame(frame);actions.grid(row=10,column=0,columnspan=2,sticky="ew",pady=18)
  self.buttons=[]
  for title,command in [("激活", "activate"),("在线验证","verify"),("本地验签","local"),("申请安装批准","request"),("释放在线租约","release")]:button=ttk.Button(actions,text=title,command=lambda command=command:self.run(command));button.pack(side="left",padx=(0,7));self.buttons.append(button)
  ttk.Label(frame,textvariable=self.status,wraplength=630).grid(row=11,column=0,columnspan=2,sticky="w",pady=12)
  ttk.Label(frame,text="安装密钥与激活缓存保存在本机持久目录；换机或换绑请走授权页面流程。\n明确拒绝会清除授权缓存。仅连接故障允许使用未到期的独立离线凭证。",wraplength=630).grid(row=12,column=0,columnspan=2,sticky="w",pady=16)
 def run(self,operation):
  environment=os.environ.copy();environment.update(YN_LICENSE_URL=self.base.get().strip(),YN_BINDING_TYPE=self.kind.get(),YN_BINDING_VALUE=self.value.get().strip(),YN_FORM="gui",YN_CARD=self.card.get().strip(),YN_INVITATION_ID=self.invite_id.get().strip(),YN_INVITATION_TOKEN=self.invite_token.get().strip(),YN_PENDING_ID=self.pending.get().strip())
  for button in self.buttons:button.state(["disabled"])
  self.status.set("正在"+operation+"…")
  def worker():
   result=subprocess.run([sys.executable,str(pathlib.Path(__file__).with_name("client.py")),operation],env=environment,capture_output=True,text=True)
   def finish():
    for button in self.buttons:button.state(["!disabled"])
    if result.returncode:self.status.set("验证未通过："+(result.stderr.strip().splitlines()[-1] if result.stderr.strip() else "客户端返回错误"));return
    response=json.loads(result.stdout.strip());self.card.set("");self.invite_token.set("")
    if response.get("offline"):text="离线宽限有效 · 仅在当前签名凭证截止前可用"
    elif response.get("released"):text="在线租约已释放"
    elif response.get("status")=="pending":text="申请已提交，请让管理员批准后填写申请 ID："+response.get("id",response.get("pending_id",""))
    else:text="授权验证通过 · 许可证 "+response.get("license_id","")
    self.status.set(text)
   self.root.after(0,finish)
  threading.Thread(target=worker,daemon=True).start()
if __name__=="__main__":root=tk.Tk();Window(root);root.mainloop()
