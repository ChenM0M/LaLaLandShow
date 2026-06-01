# La La Land 九幕图原片对齐研究

证据状态：`agent_reported`。本文件用于指导首页九幕图、后续重绘 prompt 和舞台视觉设计；不是剧情台词逐字复刻。为避免把电影截图当素材，重绘时只对齐人物、场景、道具、色彩和叙事节拍，不复刻演员真实脸。

## 资料来源

- `hard_observed`: IMDb plot summary 记录了 Mia 与 Sebastian 的主线、`Start a Fire`、Seb's 终章等关键节点。Source: https://www.imdb.com/title/tt3783958/plotsummary
- `hard_observed`: Wikipedia plot summary 记录了高架桥开场、Griffith Observatory、五年后 Seb's 等顺序节点。Source: https://en.wikipedia.org/wiki/La_La_Land
- `hard_observed`: Fashionista 对 Mary Zophres 的采访说明电影服装和美术使用 Technicolor/黄金时代歌舞片式的高饱和色彩，并用色彩追踪 Mia 与 Sebastian 的关系轨迹。Source: https://fashionista.com/2016/12/emma-stone-la-la-land-costumes
- `hard_observed`: Awards Daily 对 Mary Zophres 的采访说明开场歌舞的色彩是全片视觉的“预告”，且片中舞段有明确调色系统。Source: https://www.awardsdaily.com/2017/01/04/interview-la-la-lands-costume-designer-mary-zophres-opening-number-wanted-appetizer-rest-movie/

## 人物一致性规则

- `agent_reported`: Mia 固定为二十多岁女演员，棕色波浪短发或低盘发，纤细舞者体态；早期用高饱和裙装，后期逐步减少糖果色，终章更成熟克制。
- `agent_reported`: Sebastian 固定为二十多岁爵士钢琴手，深色短发，白衬衫、黑领带、复古西装、皮鞋是长期识别锚点；即使进入流行乐队，也保留“经典爵士人”的干净轮廓。
- `agent_reported`: 生成九幕图时不要让 Mia/Seb 每张换脸、换年龄或换体型。用同一套角色描述和同一插画风格前缀，只允许服装随剧情变化。
- `agent_reported`: 不使用 Ryan Gosling、Emma Stone 的真实肖像或电影截图作为出图要求；使用“舞台改编版 Mia / Seb”以规避肖像和截图复刻。

## 九幕对齐表

| 幕 | 原片节点 | 应出现人物 | 场景/物件 | 代表配色 | 当前风险 |
| --- | --- | --- | --- | --- | --- |
| 1 | LA 高架桥堵车与初遇，`Another Day of Sun` | Mia、Seb、通勤群舞 | 高架桥、车阵、Mia 背台词、Seb 按喇叭 | 日光金 `#F6C048`、柏油灰、红蓝黄车身 | 当前方向基本正确，需确保 Mia/Seb 只是擦肩，不是情侣同框跳舞 |
| 2 | Mia 试镜失败后被三位室友拉去派对，`Someone in the Crowd` | Mia、Alexis、Caitlin、Tracy、派对人群 | 公寓镜前换装、Hollywood Hills 派对、泳池 | 钴蓝 `#2D5CFF`、桃粉 `#F24E8A`、朱红、暖灯 | 必须保留三位室友，不应只画 Mia 独自派对 |
| 3 | 泳池派对/80s 翻唱乐队重逢 | Mia、Seb、派对乐队 | 泳池、键盘、舞台灯、停车场出口 | 泳池青 `#2FC7D9`、粉紫、夜色蓝 | 当前“海边爵士乐队”偏离；应改为泳池派对和 cover band |
| 4 | 山坡双人舞，`A Lovely Night` | Mia、Seb | 俯瞰 LA 的山坡/道路、路灯、黄裙、白衬衫黑领带 | 黄裙 `#F6C048`、暮紫 `#6157D8`、城市蓝 | 当前风格好，可继续强化黄裙与白衬衫 |
| 5 | 片场咖啡店、老影院、Griffith Observatory/Planetarium | Mia、Seb | Warner Bros. 风格片场咖啡店、Rialto 影院、天文台、星象厅 | 复古绿 `#1E7C73`、钴蓝、星光白 | 当前公寓钢琴图不对；需要重绘天文台/星象厅 |
| 6 | 恋爱蒙太奇、Hermosa Beach 码头、Lighthouse Cafe、同居与 `City of Stars` | Mia、Seb、爵士乐手 | 海边码头、爵士俱乐部、钢琴、公寓、Seb's logo 草图 | 海岸蓝 `#2FC7D9`、爵士暖金 `#D96A36`、公寓琥珀 | 可用海边/钢琴意象，但要从“相遇”改为“关系进入稳定期” |
| 7 | Seb 加入 Keith 乐队、巡演、晚餐争吵、错过 Mia 独角戏 | Seb、Keith、乐队、Mia | 流行舞台、合成器、餐桌、小剧场空席 | 电光青紫 `#445E9C`、舞台红、空剧场暖棕 | 当前与第八幕试镜混在一起；必须单独拆出 |
| 8 | Seb 带 Mia 回 LA 试镜，`Audition (The Fools Who Dream)`，随后告别 | Mia、Seb、选角导演 | Boulder City 家门口、试镜室、公园长椅/清晨 | 素白、浅蓝、清晨金 `#F4B860` | 当前“告别”方向可保留，但需要把试镜放到核心 |
| 9 | 五年后 Mia 与丈夫 David 进入 Seb's，Epilogue 梦境，最终相望 | Mia、David、Seb、爵士乐手 | Seb's 霓虹招牌、钢琴、观众席、梦境蒙太奇 | 黑金 `#233A8B`、霓虹红、梦境 Technicolor | 当前方向基本正确；需加入 David 与“不是重逢复合”的距离感 |

## 统一出图前缀

```text
Use the same stage-adaptation character bible in every image:
Mia is a young aspiring actress with chestnut-brown wavy bob or tidy updo, expressive dancer posture, no celebrity likeness. Sebastian is a young jazz pianist with dark short hair, white shirt, narrow black tie, vintage dark suit silhouette, no celebrity likeness. Maintain identical faces, body proportions, and age across all nine images. Painterly cinematic poster illustration, textured brushwork, saturated Technicolor-inspired lighting, square composition, no text, no watermark, no direct movie screenshot recreation.
```

## 九幕重绘 Prompt 摘要

1. `agent_reported`: LA freeway traffic jam turning into ensemble dance; Mia in her car rehearsing, Seb behind her in another car, commuters dancing on cars; blazing sun, asphalt, primary-color vehicles.
2. `agent_reported`: Mia and three roommates preparing for a Hollywood Hills party, then spilling into a neon pool party; blue Mia dress, red/yellow/green roommate dresses, crowd energy.
3. `agent_reported`: Pool party cover band scene; Seb reluctantly playing keyboard with an 80s cover band, Mia teasing him near the pool, stage lights and parked cars visible.
4. `agent_reported`: Hilltop `A Lovely Night`; Mia in yellow dress and Seb in white shirt/black tie dancing above Los Angeles at purple dusk under a streetlamp.
5. `agent_reported`: Griffith Observatory/planetarium romance; Mia in green dress and Seb in suit, old cinema/observatory motifs, star projector dome, deep cobalt sky.
6. `agent_reported`: Romance montage; Hermosa Beach pier, Lighthouse Cafe jazz, warm apartment piano, Seb's club-name sketch, sea blue and amber light.
7. `agent_reported`: Career divergence; Keith's pop-jazz stage with electric lights contrasted with Mia alone in a small theater after her one-woman play, no romantic glamour.
8. `agent_reported`: Audition and farewell; stark casting room, Mia singing to unseen casting directors, Seb waiting outside/supporting, later a quiet park bench in morning gold.
9. `agent_reported`: Five years later at Seb's; mature Mia with husband David in audience, Seb at piano, black-gold jazz club, dreamlike alternate-life montage hinted in light, final distant smile.

## 实施建议

- `agent_reported`: 首页文本应使用九个独立页，不再合并第六/七幕。
- `agent_reported`: 当前图片风格可以保留为目标风格，但第 3、5、7 幕内容偏差最大，优先重绘。
- `agent_reported`: 重绘时先生成一张角色 bible/reference sheet，再生成九幕；若工具无法稳定保持人物一致性，至少锁定发型、服装轮廓、体型、脸型和颜色锚点。
