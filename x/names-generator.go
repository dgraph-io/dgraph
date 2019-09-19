// Taken from https://github.com/moby/moby/blob/master/pkg/namesgenerator/names-generator.go
// Modified for use in Dgraph.
/*
                                 Apache License
                           Version 2.0, January 2004
                        https://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS

   Copyright 2013-2018 Docker, Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       https://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package x

import (
	"fmt"
	"math/rand"
)

var (
	left = [...]string{
		"admiring",
		"adoring",
		"affectionate",
		"agitated",
		"amazing",
		"angry",
		"awesome",
		"beautiful",
		"blissful",
		"bold",
		"boring",
		"brave",
		"busy",
		"charming",
		"clever",
		"cocky",
		"cool",
		"compassionate",
		"competent",
		"condescending",
		"confident",
		"cranky",
		"crazy",
		"dazzling",
		"determined",
		"distracted",
		"dreamy",
		"eager",
		"ecstatic",
		"elastic",
		"elated",
		"elegant",
		"eloquent",
		"epic",
		"exciting",
		"fervent",
		"festive",
		"flamboyant",
		"focused",
		"friendly",
		"frosty",
		"funny",
		"gallant",
		"gifted",
		"goofy",
		"gracious",
		"great",
		"happy",
		"hardcore",
		"heuristic",
		"hopeful",
		"hungry",
		"infallible",
		"inspiring",
		"interesting",
		"intelligent",
		"jolly",
		"jovial",
		"keen",
		"kind",
		"laughing",
		"loving",
		"lucid",
		"magical",
		"mystifying",
		"modest",
		"musing",
		"naughty",
		"nervous",
		"nice",
		"nifty",
		"nostalgic",
		"objective",
		"optimistic",
		"peaceful",
		"pedantic",
		"pensive",
		"practical",
		"priceless",
		"quirky",
		"quizzical",
		"recursing",
		"relaxed",
		"reverent",
		"romantic",
		"sad",
		"serene",
		"sharp",
		"silly",
		"sleepy",
		"stoic",
		"strange",
		"stupefied",
		"suspicious",
		"sweet",
		"tender",
		"thirsty",
		"trusting",
		"unruffled",
		"upbeat",
		"vibrant",
		"vigilant",
		"vigorous",
		"wizardly",
		"wonderful",
		"xenodochial",
		"youthful",
		"zealous",
		"zen",
	}

	// Docker, starting from 0.7.x, generates names from notable scientists and hackers.
	// Please, for any amazing man that you add to the list, consider adding an equally amazing
	// woman to it, and vice versa.
	right = [...]string{
		// Muhammad ibn Jābir al-Ḥarrānī al-Battānī was a founding father of astronomy.
		"albattani",

		// Frances E. Allen, became the first female IBM Fellow in 1989. In 2006, she became the
		// first female recipient of the ACM's Turing Award.
		"allen",

		// June Almeida - Scottish virologist who took the first pictures of the rubella virus.
		"almeida",

		// Kathleen Antonelli, American computer programmer and one of the six original programmers
		// of the ENIAC.
		"antonelli",

		// Maria Gaetana Agnesi - Italian mathematician, philosopher, theologian and humanitarian.
		// She was the first woman to write a mathematics handbook and the first woman appointed
		// as a Mathematics Professor at a University.
		"agnesi",

		// Archimedes was a physicist, engineer and mathematician who invented too many things to
		// list them here.
		"archimedes",

		// Maria Ardinghelli - Italian translator, mathematician and physicist.
		"ardinghelli",

		// Aryabhata - Ancient Indian mathematician-astronomer during 476-550 CE.
		"aryabhata",

		// Wanda Austin - Wanda Austin is the President and CEO of The Aerospace Corporation, a
		// leading architect for the US security space programs.
		"austin",

		// Charles Babbage invented the concept of a programmable computer.
		"babbage",

		// Stefan Banach - Polish mathematician, was one of the founders of modern functional
		// analysis.
		"banach",

		// Buckaroo Banzai and his mentor Dr. Hikita perfectd the "oscillation overthruster", a
		// device that allows one to pass through solid matter.
		"banzai",

		// John Bardeen co-invented the transistor.
		"bardeen",

		// Jean Bartik, born Betty Jean Jennings, was one of the original programmers for the ENIAC
		// computer.
		"bartik",

		// Laura Bassi, the world's first female professor.
		"bassi",

		// Hugh Beaver, British engineer, founder of the Guinness Book of World Records.
		"beaver",

		// Alexander Graham Bell - an eminent Scottish-born scientist, inventor, engineer and
		// innovator who is credited with inventing the first practical telephone.
		"bell",

		// Karl Friedrich Benz - a German automobile engineer. Inventor of the first practical
		// motorcar.
		"benz",

		// Homi J Bhabha - was an Indian nuclear physicist, founding director, and professor of
		// physics at the Tata Institute of Fundamental Research. Colloquially known as "father of
		// Indian nuclear programme".
		"bhabha",

		// Bhaskara II - Ancient Indian mathematician-astronomer whose work on calculus predates
		// Newton and Leibniz by over half a millennium.
		"bhaskara",

		// Sue Black - British computer scientist and campaigner. She has been instrumental in
		// saving Bletchley Park, the site of World War II codebreaking.
		"black",

		// Elizabeth Helen Blackburn - Australian-American Nobel laureate; best known for
		// co-discovering telomerase.
		"blackburn",

		// Elizabeth Blackwell - American doctor and first American woman to receive a medical
		// degree.
		"blackwell",

		// Niels Bohr is the father of quantum theory.
		"bohr",

		// Kathleen Booth, she's credited with writing the first assembly language.
		"booth",

		// Anita Borg - Anita Borg was the founding director of the Institute for Women and
		// Technology (IWT).
		"borg",

		// Satyendra Nath Bose - He provided the foundation for Bose–Einstein statistics and the
		// theory of the Bose–Einstein condensate. -
		"bose",

		// Katherine Louise Bouman is an imaging scientist and Assistant Professor of Computer
		// Science at the California Institute of Technology. She researches computational methods
		// for imaging, and developed an algorithm that made possible the picture first
		// visualization of a black hole using the Event Horizon Telescope.
		"bouman",

		// Evelyn Boyd Granville - She was one of the first African-American woman to receive a
		// Ph.D. in mathematics; she earned it in 1949 from Yale University.
		"boyd",

		// Brahmagupta - Ancient Indian mathematician during 598-670 CE who gave rules to compute
		// with zero.
		"brahmagupta",

		// Walter Houser Brattain co-invented the transistor.
		"brattain",

		// Emmett Brown invented time travel.
		"brown",

		// Linda Brown Buck - American biologist and Nobel laureate best known for her genetic and
		// molecular analyses of the mechanisms of smell.
		"buck",

		// Dame Susan Jocelyn Bell Burnell - Northern Irish astrophysicist who discovered radio
		// pulsars and was the first to analyse them.
		"burnell",

		// Annie Jump Cannon - pioneering female astronomer who classified hundreds of thousands of
		// stars and created the system we use to understand stars today.
		"cannon",

		// Rachel Carson - American marine biologist and conservationist, her book Silent Spring and
		// other writings are credited with advancing the global environmental movement.
		"carson",

		// Dame Mary Lucy Cartwright - British mathematician who was one of the first to study what
		// is now known as chaos theory. Also known for Cartwright's theorem which finds
		// applications in signal processing.
		"cartwright",

		// Vinton Gray Cerf - American Internet pioneer, recognised as one of "the fathers of the
		// Internet". With Robert Elliot Kahn, he designed TCP and IP, the primary data
		// communication protocols of the Internet and other computer networks.
		"cerf",

		// Subrahmanyan Chandrasekhar - Astrophysicist known for his mathematical theory on
		// different stages and evolution in structures of the stars. He has won nobel prize for
		// physics -
		"chandrasekhar",

		// Sergey Alexeyevich Chaplygin (Russian: Серге́й Алексе́евич Чаплы́гин; April 5, 1869 –
		// October 8, 1942) was a Russian and Soviet physicist, mathematician, and mechanical
		// engineer. He is known for mathematical formulas such as Chaplygin's equation and for a
		// hypothetical substance in cosmology called Chaplygin gas, named after him.
		"chaplygin",

		// Émilie du Châtelet - French natural philosopher, mathematician, physicist, and author
		// during the early 1730s, known for her translation of and commentary on Isaac Newton's
		// book Principia containing basic laws of physics.
		"chatelet",

		// Asima Chatterjee was an Indian organic chemist noted for her research on vinca alkaloids,
		// development of drugs for treatment of epilepsy and malaria -
		"chatterjee",

		// Pafnuty Chebyshev - Russian mathematician. He is known fo his works on probability,
		// statistics, mechanics, analytical geometry and number theory
		"chebyshev",

		// Bram Cohen - American computer programmer and author of the BitTorrent peer-to-peer
		// protocol.
		"cohen",

		// David Lee Chaum - American computer scientist and cryptographer. Known for his seminal
		// contributions in the field of anonymous communication.
		"chaum",

		// Joan Clarke - Bletchley Park code breaker during the Second World War who pioneered
		// techniques that remained top secret for decades. Also an accomplished numismatist.
		"clarke",

		// Jane Colden - American botanist widely considered the first female American botanist.
		"colden",

		// Gerty Theresa Cori - American biochemist who became the third woman—and first American
		// woman—to win a Nobel Prize in science, and the first woman to be awarded the Nobel Prize
		// in Physiology or Medicine. Cori was born in Prague.
		"cori",

		// Seymour Roger Cray was an American electrical engineer and supercomputer architect who
		// designed a series of computers that were the fastest in the world for decades.
		"cray",

		// This entry reflects a husband and wife team who worked together: Joan Curran was a Welsh
		// scientist who developed radar and invented chaff, a radar countermeasure. Samuel Curran
		// was an Irish physicist who worked alongside his wife during WWII and invented the
		// proximity fuse.
		"curran",

		// Marie Curie discovered radioactivity.
		"curie",

		// Charles Darwin established the principles of natural evolution.
		"darwin",

		// Leonardo Da Vinci invented too many things to list here.
		"davinci",

		// A. K. (Alexander Keewatin) Dewdney, Canadian mathematician, computer scientist, author
		// and filmmaker. Contributor to Scientific American's "Computer Recreations" from 1984 to
		// 1991. Author of Core War (program), The Planiverse, The Armchair Universe, The Magic
		// Machine, The New Turing Omnibus, and more.
		"dewdney",

		// Satish Dhawan - Indian mathematician and aerospace engineer, known for leading the
		// successful and indigenous development of the Indian space programme.
		"dhawan",

		// Bailey Whitfield Diffie - American cryptographer and one of the pioneers of public-key
		// cryptography.
		"diffie",

		// Edsger Wybe Dijkstra was a Dutch computer scientist and mathematical scientist.
		"dijkstra",

		// Paul Adrien Maurice Dirac - English theoretical physicist who made fundamental
		// contributions to the early development of both quantum mechanics and quantum
		// electrodynamics.
		"dirac",

		// Agnes Meyer Driscoll - American cryptanalyst during World Wars I and II who successfully
		// cryptanalysed a number of Japanese ciphers. She was also the co-developer of one of the
		// cipher machines of the US Navy, the CM.
		"driscoll",

		// Donna Dubinsky - played an integral role in the development of personal digital
		// assistants (PDAs) serving as CEO of Palm, Inc. and co-founding Handspring.
		"dubinsky",

		// Annie Easley - She was a leading member of the team which developed software for the
		// Centaur rocket stage and one of the first African-Americans in her field.
		"easley",

		// Thomas Alva Edison, prolific inventor.
		"edison",

		// Albert Einstein invented the general theory of relativity.
		"einstein",

		// Alexandra Asanovna Elbakyan (Russian: Алекса́ндра Аса́новна Элбакя́н) is a Kazakhstani
		// graduate student, computer programmer, internet pirate in hiding, and the creator of the
		// site Sci-Hub. Nature has listed her in 2016 in the top ten people that mattered in
		// science, and Ars Technica has compared her to Aaron Swartz.
		"elbakyan",

		// Taher A. ElGamal - Egyptian cryptographer best known for the ElGamal discrete log
		// cryptosystem and the ElGamal digital signature scheme.
		"elgamal",

		// Gertrude Elion - American biochemist, pharmacologist and the 1988 recipient of the Nobel
		// Prize in Medicine -
		"elion",

		// James Henry Ellis - British engineer and cryptographer employed by the GCHQ. Best known
		// for conceiving for the first time, the idea of public-key cryptography.
		"ellis",

		// Douglas Engelbart gave the mother of all demos.
		"engelbart",

		// Euclid invented geometry.
		"euclid",

		// Leonhard Euler invented large parts of modern mathematics.
		"euler",

		// Michael Faraday - British scientist who contributed to the study of electromagnetism and
		// electrochemistry.
		"faraday",

		// Horst Feistel - German-born American cryptographer who was one of the earliest
		// non-government researchers to study the design and theory of block ciphers. Co-developer
		// of DES and Lucifer. Feistel networks, a symmetric structure used in the construction of
		// block ciphers are named after him.
		"feistel",

		// Pierre de Fermat pioneered several aspects of modern mathematics.
		"fermat",

		// Enrico Fermi invented the first nuclear reactor.
		"fermi",

		// Richard Feynman was a key contributor to quantum mechanics and particle physics.
		"feynman",

		// Benjamin Franklin is famous for his experiments in electricity and the invention of the
		// lightning rod.
		"franklin",

		// Yuri Alekseyevich Gagarin - Soviet pilot and cosmonaut, best known as the first human to
		// journey into outer space.
		"gagarin",

		// Galileo was a founding father of modern astronomy, and faced politics and obscurantism to
		// establish scientific truth.
		"galileo",

		// Évariste Galois - French mathematician whose work laid the foundations of Galois theory
		// and group theory, two major branches of abstract algebra, and the subfield of Galois
		// connections, all while still in his late teens.
		"galois",

		// Kadambini Ganguly - Indian physician, known for being the first South Asian female
		// physician, trained in western medicine, to graduate in South Asia.
		"ganguly",

		// William Henry "Bill" Gates III is an American business magnate, philanthropist, investor,
		// computer programmer, and inventor.
		"gates",

		// Johann Carl Friedrich Gauss - German mathematician who made significant contributions to
		// many fields, including number theory, algebra, statistics, analysis, differential
		// geometry, geodesy, geophysics, mechanics, electrostatics, magnetic fields, astronomy,
		// matrix theory, and optics.
		"gauss",

		// Marie-Sophie Germain - French mathematician, physicist and philosopher. Known for her
		// work on elasticity theory, number theory and philosophy.
		"germain",

		// Adele Goldberg, was one of the designers and developers of the Smalltalk language.
		"goldberg",

		// Adele Goldstine, born Adele Katz, wrote the complete technical description for the first
		// electronic digital computer, ENIAC.
		"goldstine",

		// Shafi Goldwasser is a computer scientist known for creating theoretical foundations of
		// modern cryptography. Winner of 2012 ACM Turing Award.
		"goldwasser",

		// James Golick, all around gangster.
		"golick",

		// Jane Goodall - British primatologist, ethologist, and anthropologist who is considered to
		// be the world's foremost expert on chimpanzees.
		"goodall",

		// Stephen Jay Gould was was an American paleontologist, evolutionary biologist, and
		// historian of science. He is most famous for the theory of punctuated equilibrium -
		"gould",

		// Carolyn Widney Greider - American molecular biologist and joint winner of the 2009 Nobel
		// Prize for Physiology or Medicine for the discovery of telomerase.
		"greider",

		// Alexander Grothendieck - German-born French mathematician who became a leading figure in
		// the creation of modern algebraic geometry.
		"grothendieck",

		// Lois Haibt - American computer scientist, part of the team at IBM that developed FORTRAN.
		"haibt",

		// Margaret Hamilton - Director of the Software Engineering Division of the MIT
		// Instrumentation Laboratory, which developed on-board flight software for the Apollo space
		// program.
		"hamilton",

		// Caroline Harriet Haslett - English electrical engineer, electricity industry
		// administrator and champion of women's rights. Co-author of British Standard 1363 that
		// specifies AC power plugs and sockets used across the United Kingdom (which is widely
		// considered as one of the safest designs).
		"haslett",

		// Stephen Hawking pioneered the field of cosmology by combining general relativity and
		// quantum mechanics.
		"hawking",

		// Martin Edward Hellman - American cryptologist, best known for his invention of public-key
		// cryptography in co-operation with Whitfield Diffie and Ralph Merkle.
		"hellman",

		// Werner Heisenberg was a founding father of quantum mechanics.
		"heisenberg",

		// Grete Hermann was a German philosopher noted for her philosophical work on the
		// foundations of quantum mechanics.
		"hermann",

		// Caroline Lucretia Herschel - German astronomer and discoverer of several comets.
		"herschel",

		// Heinrich Rudolf Hertz - German physicist who first conclusively proved the existence of
		// the electromagnetic waves.
		"hertz",

		// Jaroslav Heyrovský was the inventor of the polarographic method, father of the
		// electroanalytical method, and recipient of the Nobel Prize in 1959. His main field of
		// work was polarography.
		"heyrovsky",

		// Dorothy Hodgkin was a British biochemist, credited with the development of protein
		// crystallography. She was awarded the Nobel Prize in Chemistry in 1964.
		"hodgkin",

		// Douglas R. Hofstadter is an American professor of cognitive science and author of the
		// Pulitzer Prize and American Book Award-winning work Goedel, Escher, Bach: An Eternal
		// Golden Braid in 1979. A mind-bending work which coined Hofstadter's Law: "It always takes
		// longer than you expect, even when you take into account Hofstadter's Law."
		"hofstadter",

		// Erna Schneider Hoover revolutionized modern communication by inventing a computerized
		// telephone switching method.
		"hoover",

		// Grace Hopper developed the first compiler for a computer programming language and is
		// credited with popularizing the term "debugging" for fixing computer glitches.
		"hopper",

		// Frances Hugle, she was an American scientist, engineer, and inventor who contributed to
		// the understanding of semiconductors, integrated circuitry, and the unique electrical
		// principles of microscopic materials.
		"hugle",

		// Hypatia - Greek Alexandrine Neoplatonist philosopher in Egypt who was one of the earliest
		// mothers of mathematics.
		"hypatia",

		// Teruko Ishizaka - Japanese scientist and immunologist who co-discovered the antibody
		// class Immunoglobulin E.
		"ishizaka",

		// Mary Jackson, American mathematician and aerospace engineer who earned the highest title
		// within NASA's engineering department -
		"jackson",

		// Yeong-Sil Jang was a Korean scientist and astronomer during the Joseon Dynasty; he
		// invented the first metal printing press and water gauge.
		"jang",

		// Betty Jennings - one of the original programmers of the ENIAC.
		"jennings",

		// Mary Lou Jepsen, was the founder and chief technology officer of One Laptop Per Child
		// (OLPC), and the founder of Pixel Qi.
		"jepsen",

		// Katherine Coleman Goble Johnson - American physicist and mathematician contributed to the
		// NASA.
		"johnson",

		// Irène Joliot-Curie - French scientist who was awarded the Nobel Prize for Chemistry in
		// 1935. Daughter of Marie and Pierre Curie.
		"joliot",

		// Karen Spärck Jones came up with the concept of inverse document frequency, which is used
		// in most search engines today.
		"jones",

		// A. P. J. Abdul Kalam - is an Indian scientist aka Missile Man of India for his work on
		// the development of ballistic missile and launch vehicle technology.
		"kalam",

		// Sergey Petrovich Kapitsa (Russian: Серге́й Петро́вич Капи́ца; 14 February 1928 – 14 August
		// 2012) was a Russian physicist and demographer. He was best known as host of the popular
		// and long-running Russian scientific TV show, Evident, but Incredible. His father was the
		// Nobel laureate Soviet-era physicist Pyotr Kapitsa, and his brother was the geographer and
		// Antarctic explorer Andrey Kapitsa.
		"kapitsa",

		// Susan Kare, created the icons and many of the interface elements for the original Apple
		// Macintosh in the 1980s, and was an original employee of NeXT, working as the Creative
		// Director.
		"kare",

		// Mstislav Keldysh - a Soviet scientist in the field of mathematics and mechanics,
		// academician of the USSR Academy of Sciences (1946), President of the USSR Academy of
		// Sciences (1961–1975), three times Hero of Socialist Labor (1956, 1961, 1971), fellow of
		// the Royal Society of Edinburgh (1968).
		"keldysh",

		// Mary Kenneth Keller, Sister Mary Kenneth Keller became the first American woman to earn a
		// PhD in Computer Science in 1965.
		"keller",

		// Johannes Kepler, German astronomer known for his three laws of planetary motion -
		"kepler",

		// Omar Khayyam - Persian mathematician, astronomer and poet. Known for his work on the
		// classification and solution of cubic equations, for his contribution to the understanding
		// of Euclid's fifth postulate and for computing the length of a year very accurately.
		"khayyam",

		// Har Gobind Khorana - Indian-American biochemist who shared the 1968 Nobel Prize for
		// Physiology.
		"khorana",

		// Jack Kilby invented silicone integrated circuits and gave Silicon Valley its name.
		"kilby",

		// Maria Kirch - German astronomer and first woman to discover a comet.
		"kirch",

		// Donald Knuth - American computer scientist, author of "The Art of Computer Programming"
		// and creator of the TeX typesetting system.
		"knuth",

		// Sophie Kowalevski - Russian mathematician responsible for important original
		// contributions to analysis, differential equations and mechanics.
		"kowalevski",

		// Marie-Jeanne de Lalande - French astronomer, mathematician and cataloguer of stars.
		"lalande",

		// Hedy Lamarr - Actress and inventor. The principles of her work are now incorporated into
		// modern Wi-Fi, CDMA and Bluetooth technology.
		"lamarr",

		// Leslie B. Lamport - American computer scientist. Lamport is best known for his seminal
		// work in distributed systems and was the winner of the 2013 Turing Award.
		"lamport",

		// Mary Leakey - British paleoanthropologist who discovered the first fossilized Proconsul
		// skull.
		"leakey",

		// Henrietta Swan Leavitt - she was an American astronomer who discovered the relation
		// between the luminosity and the period of Cepheid variable stars.
		"leavitt",

		// Esther Miriam Zimmer Lederberg - American microbiologist and a pioneer of bacterial
		// genetics.
		"lederberg",

		// Inge Lehmann - Danish seismologist and geophysicist. Known for discovering in 1936 that
		// the Earth has a solid inner core inside a molten outer core.
		"lehmann",

		// Daniel Lewin - Mathematician, Akamai co-founder, soldier, 9/11 victim-- Developed
		// optimization techniques for routing traffic on the internet. Died attempting to stop the
		// 9-11 hijackers.
		"lewin",

		// Ruth Lichterman - one of the original programmers of the ENIAC.
		"lichterman",

		// Barbara Liskov - co-developed the Liskov substitution principle. Liskov was also the
		// winner of the Turing Prize in 2008.
		"liskov",

		// Ada Lovelace invented the first algorithm.
		"lovelace",

		// Auguste and Louis Lumière - the first filmmakers in history.
		"lumiere",

		// Mahavira - Ancient Indian mathematician during 9th century AD who discovered basic
		// algebraic identities -
		"mahavira",

		// Lynn Margulis (b. Lynn Petra Alexander) - an American evolutionary theorist and
		// biologist, science author, educator, and popularizer, and was the primary modern
		// proponent for the significance of symbiosis in evolution.
		"margulis",

		// Yukihiro Matsumoto - Japanese computer scientist and software programmer best known as
		// the chief designer of the Ruby programming language.
		"matsumoto",

		// James Clerk Maxwell - Scottish physicist, best known for his formulation of
		// electromagnetic theory.
		"maxwell",

		// Maria Mayer - American theoretical physicist and Nobel laureate in Physics for proposing
		// the nuclear shell model of the atomic nucleus.
		"mayer",

		// John McCarthy invented LISP.
		"mccarthy",

		// Barbara McClintock - a distinguished American cytogeneticist, 1983 Nobel Laureate in
		// Physiology or Medicine for discovering transposons.
		"mcclintock",

		// Anne Laura Dorinthea McLaren - British developmental biologist whose work helped lead to
		// human in-vitro fertilisation.
		"mclaren",

		// Malcolm McLean invented the modern shipping container.
		"mclean",

		// Kay McNulty - one of the original programmers of the ENIAC.
		"mcnulty",

		// Gregor Johann Mendel - Czech scientist and founder of genetics.
		"mendel",

		// Dmitri Mendeleev - a chemist and inventor. He formulated the Periodic Law, created a
		// farsighted version of the periodic table of elements, and used it to correct the
		// properties of some already discovered elements and also to predict the properties of
		// eight elements yet to be discovered.
		"mendeleev",

		// Lise Meitner - Austrian/Swedish physicist who was involved in the discovery of nuclear
		// fission. The element meitnerium is named after her.
		"meitner",

		// Carla Meninsky, was the game designer and programmer for Atari 2600 games Dodge 'Em and
		// Warlords.
		"meninsky",

		// Ralph C. Merkle - American computer scientist, known for devising Merkle's puzzles - one
		// of the very first schemes for public-key cryptography. Also, inventor of Merkle trees and
		// co-inventor of the Merkle-Damgård construction for building collision-resistant
		// cryptographic hash functions and the Merkle-Hellman knapsack cryptosystem.
		"merkle",

		// Johanna Mestorf - German prehistoric archaeologist and first female museum director in
		// Germany.
		"mestorf",

		// Marvin Minsky - Pioneer in Artificial Intelligence, co-founder of the MIT's AI Lab, won
		// the Turing Award in 1969.
		"minsky",

		// Maryam Mirzakhani - an Iranian mathematician and the first woman to win the Fields Medal.
		"mirzakhani",

		// Gordon Earle Moore - American engineer, Silicon Valley founding father, author of Moore's
		// law.
		"moore",

		// Samuel Morse - contributed to the invention of a single-wire telegraph system based on
		// European telegraphs and was a co-developer of the Morse code.
		"morse",

		// Ian Murdock - founder of the Debian project.
		"murdock",

		// May-Britt Moser - Nobel prize winner neuroscientist who contributed to the discovery of
		// grid cells in the brain.
		"moser",

		// John Napier of Merchiston - Scottish landowner known as an astronomer, mathematician and
		// physicist. Best known for his discovery of logarithms.
		"napier",

		// John Forbes Nash, Jr. - American mathematician who made fundamental contributions to game
		// theory, differential geometry, and the study of partial differential equations.
		"nash",

		// John von Neumann - todays computer architectures are based on the von Neumann
		// architecture.
		"neumann",

		// Isaac Newton invented classic mechanics and modern optics.
		"newton",

		// Florence Nightingale, more prominently known as a nurse, was also the first female member
		// of the Royal Statistical Society and a pioneer in statistical graphics.
		"nightingale",

		// Alfred Nobel - a Swedish chemist, engineer, innovator, and armaments manufacturer
		// (inventor of dynamite).
		"nobel",

		// Emmy Noether, German mathematician. Noether's Theorem is named after her.
		"noether",

		// Poppy Northcutt. Poppy Northcutt was the first woman to work as part of NASA’s Mission
		// Control.
		"northcutt",

		// Robert Noyce invented silicone integrated circuits and gave Silicon Valley its name.
		"noyce",

		// Panini - Ancient Indian linguist and grammarian from 4th century CE who worked on the
		// world's first formal system.
		"panini",

		// Ambroise Pare invented modern surgery.
		"pare",

		// Blaise Pascal, French mathematician, physicist, and inventor.
		"pascal",

		// Louis Pasteur discovered vaccination, fermentation and pasteurization.
		"pasteur",

		// Cecilia Payne-Gaposchkin was an astronomer and astrophysicist who, in 1925, proposed in
		// her Ph.D. thesis an explanation for the composition of stars in terms of the relative
		// abundances of hydrogen and helium.
		"payne",

		// Radia Perlman is a software designer and network engineer and most famous for her
		// invention of the spanning-tree protocol (STP).
		"perlman",

		// Rob Pike was a key contributor to Unix, Plan 9, the X graphic system, utf-8, and the Go
		// programming language.
		"pike",

		// Henri Poincaré made fundamental contributions in several fields of mathematics.
		"poincare",

		// Laura Poitras is a director and producer whose work, made possible by open source crypto
		// tools, advances the causes of truth and freedom of information by reporting disclosures
		// by whistleblowers such as Edward Snowden.
		"poitras",

		// Tat’yana Avenirovna Proskuriakova (Russian: Татья́на Авени́ровна Проскуряко́ва) (January 23
		// [O.S. January 10] 1909 – August 30, 1985) was a Russian-American Mayanist scholar and
		// archaeologist who contributed significantly to the deciphering of Maya hieroglyphs, the
		// writing system of the pre-Columbian Maya civilization of Mesoamerica.
		"proskuriakova",

		// Claudius Ptolemy - a Greco-Egyptian writer of Alexandria, known as a mathematician,
		// astronomer, geographer, astrologer, and poet of a single epigram in the Greek Anthology.
		"ptolemy",

		// C. V. Raman - Indian physicist who won the Nobel Prize in 1930 for proposing the Raman
		// effect.
		"raman",

		// Srinivasa Ramanujan - Indian mathematician and autodidact who made extraordinary
		// contributions to mathematical analysis, number theory, infinite series, and continued
		// fractions.
		"ramanujan",

		// Sally Kristen Ride was an American physicist and astronaut. She was the first American
		// woman in space, and the youngest American astronaut.
		"ride",

		// Rita Levi-Montalcini - Won Nobel Prize in Physiology or Medicine jointly with colleague
		// Stanley Cohen for the discovery of nerve growth factor.
		"montalcini",

		// Dennis Ritchie - co-creator of UNIX and the C programming language.
		"ritchie",

		// Ida Rhodes - American pioneer in computer programming, designed the first computer used
		// for Social Security.
		"rhodes",

		// Julia Hall Bowman Robinson - American mathematician renowned for her contributions to the
		// fields of computability theory and computational complexity theory.
		"robinson",

		// Wilhelm Conrad Röntgen - German physicist who was awarded the first Nobel Prize in
		// Physics in 1901 for the discovery of X-rays (Röntgen rays).
		"roentgen",

		// Rosalind Franklin - British biophysicist and X-ray crystallographer whose research was
		// critical to the understanding of DNA -
		"rosalind",

		// Vera Rubin - American astronomer who pioneered work on galaxy rotation rates.
		"rubin",

		// Meghnad Saha - Indian astrophysicist best known for his development of the Saha equation,
		// used to describe chemical and physical conditions in stars -
		"saha",

		// Jean E. Sammet developed FORMAC, the first widely used computer language for symbolic
		// manipulation of mathematical formulas.
		"sammet",

		// Mildred Sanderson - American mathematician best known for Sanderson's theorem concerning
		// modular invariants.
		"sanderson",

		// Satoshi Nakamoto is the name used by the unknown person or group of people who developed
		// bitcoin, authored the bitcoin white paper, and created and deployed bitcoin's original
		// reference implementation.
		"satoshi",

		// Adi Shamir - Israeli cryptographer whose numerous inventions and contributions to
		// cryptography include the Ferge Fiat Shamir identification scheme, the Rivest Shamir
		// Adleman (RSA) public-key cryptosystem, the Shamir's secret sharing scheme, the breaking
		// of the Merkle-Hellman cryptosystem, the TWINKLE and TWIRL factoring devices and the
		// discovery of differential cryptanalysis (with Eli Biham).
		"shamir",

		// Claude Shannon - The father of information theory and founder of digital circuit design
		// theory.
		"shannon",

		// Carol Shaw - Originally an Atari employee, Carol Shaw is said to be the first female
		// video game designer.
		"shaw",

		// Dame Stephanie "Steve" Shirley - Founded a software company in 1962 employing women
		// working from home.
		"shirley",

		// William Shockley co-invented the transistor -
		"shockley",

		// Lina Solomonovna Stern (or Shtern; Russian: Лина Соломоновна Штерн; 26 August 1878 – 7
		// March 1968) was a Soviet biochemist, physiologist and humanist whose medical discoveries
		// saved thousands of lives at the fronts of World War II. She is best known for her
		// pioneering work on blood–brain barrier, which she described as hemato-encephalic barrier
		// in 1921.
		"shtern",

		// Françoise Barré-Sinoussi - French virologist and Nobel Prize Laureate in Physiology or
		// Medicine; her work was fundamental in identifying HIV as the cause of AIDS.
		"sinoussi",

		// Betty Snyder - one of the original programmers of the ENIAC.
		"snyder",

		// Cynthia Solomon - Pioneer in the fields of artificial intelligence, computer science and
		// educational computing. Known for creation of Logo, an educational programming language.
		"solomon",

		// Frances Spence - one of the original programmers of the ENIAC.
		"spence",

		// Richard Matthew Stallman - the founder of the Free Software movement, the GNU project,
		// the Free Software Foundation, and the League for Programming Freedom. He also invented
		// the concept of copyleft to protect the ideals of this movement, and enshrined this
		// concept in the widely-used GPL (General Public License) for software.
		"stallman",

		// Michael Stonebraker is a database research pioneer and architect of Ingres, Postgres,
		// VoltDB and SciDB. Winner of 2014 ACM Turing Award.
		"stonebraker",

		// Ivan Edward Sutherland - American computer scientist and Internet pioneer, widely
		// regarded as the father of computer graphics.
		"sutherland",

		// Janese Swanson (with others) developed the first of the Carmen Sandiego games. She went
		// on to found Girl Tech.
		"swanson",

		// Aaron Swartz was influential in creating RSS, Markdown, Creative Commons, Reddit, and
		// much of the internet as we know it today. He was devoted to freedom of information on the
		// web.
		"swartz",

		// Bertha Swirles was a theoretical physicist who made a number of contributions to early
		// quantum theory.
		"swirles",

		// Helen Brooke Taussig - American cardiologist and founder of the field of paediatric
		// cardiology.
		"taussig",

		// Valentina Tereshkova is a Russian engineer, cosmonaut and politician. She was the first
		// woman to fly to space in 1963. In 2013, at the age of 76, she offered to go on a one-way
		// mission to Mars.
		"tereshkova",

		// Nikola Tesla invented the AC electric system and every gadget ever used by a James Bond
		// villain.
		"tesla",

		// Marie Tharp - American geologist and oceanic cartographer who co-created the first
		// scientific map of the Atlantic Ocean floor. Her work led to the acceptance of the
		// theories of plate tectonics and continental drift.
		"tharp",

		// Ken Thompson - co-creator of UNIX and the C programming language.
		"thompson",

		// Linus Torvalds invented Linux and Git.
		"torvalds",

		// Youyou Tu - Chinese pharmaceutical chemist and educator known for discovering artemisinin
		// and dihydroartemisinin, used to treat malaria, which has saved millions of lives. Joint
		// winner of the 2015 Nobel Prize in Physiology or Medicine.
		"tu",

		// Alan Turing was a founding father of computer science.
		"turing",

		// Varahamihira - Ancient Indian mathematician who discovered trigonometric formulae during
		// 505-587 CE.
		"varahamihira",

		// Dorothy Vaughan was a NASA mathematician and computer programmer on the SCOUT launch
		// vehicle program that put America's first satellites into space.
		"vaughan",

		// Sir Mokshagundam Visvesvaraya - is a notable Indian engineer. He is a recipient of the
		// Indian Republic's highest honour, the Bharat Ratna, in 1955. On his birthday, 15
		// September is celebrated as Engineer's Day in India in his memory.
		"visvesvaraya",

		// Christiane Nüsslein-Volhard - German biologist, won Nobel Prize in Physiology or Medicine
		// in 1995 for research on the genetic control of embryonic development.
		"volhard",

		// Cédric Villani - French mathematician, won Fields Medal, Fermat Prize and Poincaré Price
		// for his work in differential geometry and statistical mechanics.
		"villani",

		// Marlyn Wescoff - one of the original programmers of the ENIAC.
		"wescoff",

		// Sylvia B. Wilbur - British computer scientist who helped develop the ARPANET, was one of
		// the first to exchange email in the UK and a leading researcher in computer-supported
		// collaborative work.
		"wilbur",

		// Andrew Wiles - Notable British mathematician who proved the enigmatic Fermat's Last
		// Theorem.
		"wiles",

		// Roberta Williams, did pioneering work in graphical adventure games for personal
		// computers, particularly the King's Quest series.
		"williams",

		// Malcolm John Williamson - British mathematician and cryptographer employed by the GCHQ.
		// Developed in 1974 what is now known as Diffie-Hellman key exchange (Diffie and Hellman
		// first published the scheme in 1976).
		"williamson",

		// Sophie Wilson designed the first Acorn Micro-Computer and the instruction set for ARM
		// processors.
		"wilson",

		// Jeannette Wing - co-developed the Liskov substitution principle.
		"wing",

		// Steve Wozniak invented the Apple I and Apple II.
		"wozniak",

		// The Wright brothers, Orville and Wilbur - credited with inventing and building the
		// world's first successful airplane and making the first controlled, powered and sustained
		// heavier-than-air human flight -
		"wright",

		// Chien-Shiung Wu - Chinese-American experimental physicist who made significant
		// contributions to nuclear physics.
		"wu",

		// Rosalyn Sussman Yalow - Rosalyn Sussman Yalow was an American medical physicist, and a
		// co-winner of the 1977 Nobel Prize in Physiology or Medicine for development of the
		// radioimmunoassay technique.
		"yalow",

		// Ada Yonath - an Israeli crystallographer, the first woman from the Middle East to win a
		// Nobel prize in the sciences.
		"yonath",

		// Nikolay Yegorovich Zhukovsky (Russian: Никола́й Его́рович Жуко́вский, January 17 1847 –
		// March 17, 1921) was a Russian scientist, mathematician and engineer, and a founding
		// father of modern aero- and hydrodynamics. Whereas contemporary scientists scoffed at the
		// idea of human flight, Zhukovsky was the first to undertake the study of airflow. He is
		// often called the Father of Russian Aviation.
		"zhukovsky",
	}
)

// GetRandomName generates a random name from the list of adjectives and surnames in this package
// formatted as "adjective_surname". For example 'focused_turing'. If retry is non-zero, a random
// integer between 0 and 10 will be added to the end of the name, e.g `focused_turing3`
func GetRandomName(retry int) string {
begin:
	name := fmt.Sprintf("%s_%s", left[rand.Intn(len(left))], right[rand.Intn(len(right))])
	if name == "boring_wozniak" /* Steve Wozniak is not boring */ {
		goto begin
	}

	if retry > 0 {
		name = fmt.Sprintf("%s%d", name, rand.Intn(10))
	}
	return name
}
